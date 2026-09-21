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

// runInitEnv will drive the interactive/agent flow for exporting the
// per-system password environment variables. Not implemented yet — that is
// a later task; this stub only keeps `zosmf-mcp init env` from being an
// undefined dispatch target.
func runInitEnv(args []string, stdout, stderr io.Writer, isTTY bool) int {
	_, _ = fmt.Fprintln(stderr, "zosmf-mcp init env is not implemented yet")
	return 2
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
