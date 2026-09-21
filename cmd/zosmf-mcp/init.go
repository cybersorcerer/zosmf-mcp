package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cybersorcerer/zosmf-mcp/internal/config"
	"github.com/cybersorcerer/zosmf-mcp/internal/setup"
)

// runInit handles `zosmf-mcp init …`. It returns the exit code instead of
// calling os.Exit so the tests can drive it in process.
func runInit(args []string, stdout, stderr io.Writer, isTTY bool) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: zosmf-mcp init <config|env> [flags]")
		return 2
	}
	switch args[0] {
	case "config":
		return runInitConfig(args[1:], stdout, stderr, isTTY)
	case "env":
		return runInitEnv(args[1:], stdout, stderr, isTTY)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown init subcommand %q — use config or env\n", args[0])
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
	envFile := fs.String("env-file", ".env", "environment file to write")
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
	cfg, err := config.Load(path)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\nfix the configuration first: zosmf-mcp init config --check --file %s\n", err, path)
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

	if *questions {
		fields := make([]setup.Field, 0, len(needed))
		for _, name := range needed {
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

	vars := map[string]string{}
	for _, name := range needed {
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
	return 0
}

func runInitConfig(args []string, stdout, stderr io.Writer, isTTY bool) int {
	fs := flag.NewFlagSet("init config", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "configuration file to create or update")
	questions := fs.Bool("questions", false, "print the field catalog as JSON and exit")
	print := fs.Bool("print", false, "render the result to stdout without writing")
	check := fs.Bool("check", false, "load the file through the server's config loader and report problems")

	fields := setup.ConfigFields()
	values := setup.BindFlags(fs, fields)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *questions {
		data, err := setup.CatalogJSON("init config", fields)
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

	if *check {
		if _, err := config.Load(path); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "%s is valid\n", path)
		return 0
	}

	anyFlag := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "file", "questions", "print", "check":
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
		if err := setup.AskAll(fields, values, os.Stdin, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if err := setup.Validate(fields, values); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 2
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
	if _, err := doc.UpsertSystem(values.One("name"), entries); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	if *print {
		out, err := doc.Bytes()
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
	if err := doc.Save(path); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "wrote %s\n", path)
	_, _ = fmt.Fprintf(stdout, "next: zosmf-mcp init env --file %s\n", path)
	return 0
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
