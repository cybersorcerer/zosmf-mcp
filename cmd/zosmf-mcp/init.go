package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cybersorcerer/zosmf-mcp/agents"
	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/setup"
)

// runInit handles `zosmf-mcp init …`. It returns the exit code instead of
// calling os.Exit so the tests can drive it in process.
func runInit(args []string, stdout, stderr io.Writer, isTTY bool) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: zosmf-mcp init <config|env|restore|skill> [flags]")
		return 2
	}
	switch args[0] {
	case "config":
		return runInitConfig(args[1:], stdout, stderr, isTTY)
	case "env":
		return runInitEnv(args[1:], stdout, stderr, isTTY)
	case "restore":
		return runInitRestore(args[1:], stdout, stderr)
	case "skill":
		return runInitSkill(args[1:], stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown init subcommand %q — use config, env, restore or skill\n", args[0])
		return 2
	}
}

// runInitEnv works out which secrets the configuration at --file needs and
// writes them to --env-file. It never prints a secret: not the values asked
// for, not a generated key, not an error message.
func runInitEnv(args []string, stdout, stderr io.Writer, isTTY bool) int {
	fs := flag.NewFlagSet("init env", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "configuration file to read the required variables from")
	envFile := fs.String("env-file", "", "environment file to write (default: .env next to the configuration file)")
	questions := fs.Bool("questions", false, "print the required variables as JSON and exit")
	genKey := fs.Bool("generate-api-key", false, "generate MCP_API_KEY instead of asking for it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	path, err := resolveConfigPath(*file)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if *envFile == "" {
		*envFile = envPathFor(path)
	}
	cfg, err := config.Load(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\nwrite the configuration first: zosmf-mcp init config --file %s\n", err, path)
		return 1
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	needed := setup.RequiredEnvVars(cfg, raw)

	// --generate-api-key is an explicit request for a key. RequiredEnvVars
	// only asks for MCP_API_KEY when an HTTP transport is already enabled in
	// the file, which would otherwise make the flag a no-op for a config
	// that has not turned HTTP on yet.
	if *genKey {
		hasKey := false
		for _, name := range needed {
			if name == "MCP_API_KEY" {
				hasKey = true
				break
			}
		}
		if !hasKey {
			needed = append(needed, "MCP_API_KEY")
		}
	}

	// A variable the file already holds is kept: adding a system must not
	// ask again for every other system's password. One exported in the
	// environment, or a key asked for with --generate-api-key, replaces it.
	inFile, err := setup.ReadEnvFile(*envFile)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	var kept, todo []string
	for _, name := range needed {
		generate := name == "MCP_API_KEY" && *genKey
		if inFile[name] != "" && !generate && os.Getenv(name) == "" {
			kept = append(kept, name)
		} else {
			todo = append(todo, name)
		}
	}

	if *questions {
		fields := make([]setup.Field, 0, len(todo))
		for _, name := range todo {
			if name == "MCP_API_KEY" && *genKey {
				continue
			}
			fields = append(fields, setup.Field{
				ID: name, YAMLKey: "", Label: name, Group: "env",
				Kind: setup.KindString, Cardinality: setup.One, Required: true,
				Help: "Secret. Written to the environment file, never to the configuration.",
			})
		}
		data, err := setup.CatalogJSON("init env", fields)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, string(data))
		return 0
	}

	if len(kept) > 0 {
		_, _ = fmt.Fprintf(stdout, "kept from %s: %s\n", *envFile, strings.Join(kept, ", "))
	}
	if len(todo) == 0 {
		_, _ = fmt.Fprintf(stdout, "%s already has every variable this configuration needs — nothing to do\n", *envFile)
		return 0
	}

	vars := map[string]string{}
	for _, name := range todo {
		if name == "MCP_API_KEY" && *genKey {
			key, err := setup.GenerateAPIKey()
			if err != nil {
				_, _ = fmt.Fprintln(stderr, err)
				return 1
			}
			vars[name] = key
			continue
		}
		if value := os.Getenv(name); value != "" {
			vars[name] = value
			continue
		}
		if !isTTY {
			_, _ = fmt.Fprintf(stderr, "%s is not set and there is no terminal to ask on.\n", name)
			_, _ = fmt.Fprintf(stderr, "export it and run again, or use --generate-api-key for MCP_API_KEY.\n")
			return 2
		}
		value, err := setup.AskSecret(name, os.Stdin, stdout)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		vars[name] = value
	}

	if err := setup.WriteEnvFile(*envFile, vars); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s with %d variable(s), mode 600\n", *envFile, len(vars))
	_, _ = fmt.Fprintln(stdout, "values are not printed here — read them from the file if you need them")
	_, _ = fmt.Fprintf(stdout, "make it reach the server: shell — `set -a; . %s; set +a` before starting it;\n", *envFile)
	_, _ = fmt.Fprintln(stdout, "stdio client — pass each variable with its own env flag, e.g. `claude mcp add --env NAME=value ...`")
	return 0
}

func runInitConfig(args []string, stdout, stderr io.Writer, isTTY bool) int {
	fs := flag.NewFlagSet("init config", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "configuration file to create or update")
	questions := fs.Bool("questions", false, "print the field catalog as JSON and exit")
	print := fs.Bool("print", false, "show the system as it would be written, without writing")
	check := fs.Bool("check", false, "load the file through the server's config loader and report problems")
	show := fs.Bool("show", false, "print the systems already in the file as JSON and exit")

	fields := setup.ConfigFields()
	sshFields := setup.SSHFields()
	values := setup.BindFlags(fs, append(append([]setup.Field{}, fields...), sshFields...))
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *questions {
		data, err := setup.CatalogJSON("init config", append(append([]setup.Field{}, fields...), sshFields...))
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, string(data))
		return 0
	}

	path, err := resolveConfigPath(*file)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	if *show {
		doc, err := setup.LoadDocument(path)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		type shownSystem struct {
			Name   string         `json:"name"`
			Values map[string]any `json:"values"`
		}
		systems := []shownSystem{}
		for _, s := range doc.Systems() {
			v := doc.SystemValues(s.Name, fields, sshFields)
			values := map[string]any{}
			for _, f := range append(append([]setup.Field{}, fields...), sshFields...) {
				switch {
				case f.ID == "name" || !v.Has(f.ID):
				case f.Cardinality == setup.Many:
					values[f.ID] = v.Many(f.ID)
				default:
					values[f.ID] = v.One(f.ID)
				}
			}
			systems = append(systems, shownSystem{Name: s.Name, Values: values})
		}
		data, err := json.MarshalIndent(struct {
			File    string        `json:"file"`
			Systems []shownSystem `json:"systems"`
		}{path, systems}, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, string(data))
		return 0
	}

	if *check {
		cfg, err := config.Load(path)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		bad := false
		for _, sys := range cfg.Systems {
			if err := config.CheckHost(sys.Host); err != nil {
				_, _ = fmt.Fprintf(stderr, "system %s: %v\n", sys.Name, err)
				bad = true
			}
		}
		if bad {
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "%s is valid\n", path)
		for _, sys := range cfg.Systems {
			_, _ = fmt.Fprintf(stdout, "system %s: password from %s\n", sys.Name, sys.PasswordSource)
		}
		return 0
	}

	anyFlag := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "file", "questions", "print", "check", "show":
		default:
			anyFlag = true
		}
	})

	if !anyFlag {
		if !isTTY {
			_, _ = fmt.Fprintln(stderr, "no field flags given and no terminal to ask on.")
			_, _ = fmt.Fprintln(stderr, "run `zosmf-mcp init config --questions` for the field list, then pass them as flags, e.g.:")
			_, _ = fmt.Fprintln(stderr, "  zosmf-mcp init config --name MAINT --host https://zos.example.com:443 --user IBMUSER")
			return 2
		}
		if err := setup.RunWizard(path, envPathFor(path), *print, os.Stdin, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	if err := setup.Validate(fields, values); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	// The ZOAU fields are required as a group: a user who wants no ZOAU
	// tools must be able to run without any of them, so this only fires
	// once the group has been engaged by a flag.
	zoauEngaged := false
	for _, f := range sshFields {
		if values.Has(f.ID) {
			zoauEngaged = true
			break
		}
	}
	if zoauEngaged {
		if err := setup.Validate(sshFields, values); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
	}

	entries, err := setup.EntriesFor(fields, values)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
	}

	doc, err := setup.LoadDocument(path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	existed := doc.HasSystem(values.One("name"))
	if _, err := doc.UpsertSystem(values.One("name"), entries); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	if zoauEngaged {
		sshEntries, err := setup.EntriesFor(sshFields, values)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 2
		}
		sshEntries = append(sshEntries, setup.ZOAUEnvironment(values.One("zoau-path"))...)
		if _, err := doc.UpsertSSHSystem(values.One("name"), sshEntries); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}

	// A file with no transport key at all cannot start the server. Fill it
	// with the common default rather than leaving a wizard-written
	// configuration unable to run without an extra flag; any existing
	// transport block, of any shape, is left untouched.
	if _, err := doc.EnsureDefaultTransport(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	if *print {
		out, err := doc.Preview(values.One("name"))
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprint(stdout, string(out))
		return 0
	}

	// The directory may not exist yet — most likely the first run for this
	// user. 0700 because this holds configuration for a system that grants
	// z/OS access.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	backup, err := doc.Save(path)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", path)
	if existed {
		_, _ = fmt.Fprintf(stdout, "updated system %s\n", values.One("name"))
	} else {
		_, _ = fmt.Fprintf(stdout, "added system %s\n", values.One("name"))
	}
	if backup != nil {
		_, _ = fmt.Fprintf(stdout, "previous version saved as %s\n", backup.Path)
	}
	_, _ = fmt.Fprintf(stdout, "next: zosmf-mcp init env --file %s\n", path)
	return 0
}

// targetBackup is a backup labelled with the --target that restores it.
type targetBackup struct {
	Target string `json:"target"`
	setup.Backup
}

// runInitRestore lists the backups of the configuration and .env, or puts
// one back. The version it replaces is backed up first.
func runInitRestore(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init restore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "configuration file whose backups to use")
	envFile := fs.String("env-file", "", "environment file whose backups to use (default: .env next to the configuration file)")
	list := fs.Bool("list", false, "print the backups as JSON, newest first, and exit")
	stamp := fs.String("backup", "", "timestamp of the backup to restore, as --list prints it")
	target := fs.String("target", "", "config or env — needed only when both have a backup with that timestamp")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if !*list && *stamp == "" {
		_, _ = fmt.Fprintln(stderr, "usage: zosmf-mcp init restore --list | --backup <timestamp> [--target config|env]")
		return 2
	}

	path, err := resolveConfigPath(*file)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if *envFile == "" {
		*envFile = envPathFor(path)
	}
	var all []targetBackup
	for _, t := range []struct{ name, path string }{{"config", path}, {"env", *envFile}} {
		backups, err := setup.ListBackups(t.path)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		for _, b := range backups {
			all = append(all, targetBackup{Target: t.name, Backup: b})
		}
	}

	if *list {
		if all == nil {
			all = []targetBackup{}
		}
		data, err := json.MarshalIndent(struct {
			Backups []targetBackup `json:"backups"`
		}{all}, "", "  ")
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintln(stdout, string(data))
		return 0
	}

	var matches []targetBackup
	for _, b := range all {
		if b.Stamp == *stamp && (*target == "" || *target == b.Target) {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 0:
		_, _ = fmt.Fprintf(stderr, "no backup %s — see zosmf-mcp init restore --list\n", *stamp)
		return 1
	case 1:
	default:
		_, _ = fmt.Fprintf(stderr, "both the config and .env have a backup %s — add --target config or --target env\n", *stamp)
		return 2
	}
	b := matches[0]
	saved, err := setup.RestoreBackup(b.File, b.Stamp)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "restored %s from the backup of %s\n", b.File, b.Stamp)
	if saved != nil {
		_, _ = fmt.Fprintf(stdout, "the version it replaced is saved as %s\n", saved.Path)
	}
	return 0
}

// skillDirs is where each agent looks for skills, per scope. A project
// scope is relative to the directory the command runs in.
var skillDirs = map[string]map[string]string{
	"claude":   {"project": ".claude/skills", "user": ".claude/skills"},
	"opencode": {"project": ".opencode/skills", "user": ".config/opencode/skills"},
	"pi":       {"project": ".pi/skills", "user": ".pi/agent/skills"},
}

// runInitSkill copies the setup procedure, and the agent's wrapper for it,
// out of the binary into that agent's skill directory.
func runInitSkill(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init skill", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "claude", "claude, opencode or pi")
	scope := fs.String("scope", "user", "user (all projects) or project (the current directory)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	base, ok := skillDirs[*agent][*scope]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "unknown --agent %q or --scope %q — use claude, opencode or pi, and user or project\n", *agent, *scope)
		return 2
	}
	if *scope == "user" {
		home, err := os.UserHomeDir()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		base = filepath.Join(home, base)
	}
	dir := filepath.Join(base, "zosmf-mcp-setup")

	// The wrapper and setup.md go together: the wrapper refers to the file
	// next to it, and one without the other leaves an agent pointing at
	// nothing.
	files := map[string]string{"SKILL.md": *agent + "/SKILL.md", "setup.md": "setup.md"}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	for name, src := range files {
		data, err := agents.Files.ReadFile(src)
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, name), data, 0o644)
		}
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	_, _ = fmt.Fprintf(stdout, "installed the zosmf-mcp setup skill for %s (%s):\n  %s\n", *agent, *scope, dir)
	return 0
}

// envPathFor keeps the secrets beside the configuration they belong to,
// wherever the command is run from.
func envPathFor(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), ".env")
}

// resolveConfigPath honours --file, then the server's own search order, and
// falls back to the shared location when nothing exists yet.
func resolveConfigPath(fileFlag string) (string, error) {
	if fileFlag != "" {
		return fileFlag, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	path, _, err := config.ResolveDefaultPath(cwd, home)
	if err == nil {
		return path, nil
	}
	if home == "" {
		return "", fmt.Errorf("no configuration found and no home directory to put one in — pass --file")
	}
	return filepath.Join(home, ".config", "smpe_ls", config.DefaultConfigName), nil
}
