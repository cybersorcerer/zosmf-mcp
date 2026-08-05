package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/tools"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	configPath := flag.String("config", "", "Path to configuration file (default: search .smpe-zosmf.yaml in cwd and ~/.config/smpe_ls/)")
	transport := flag.String("transport", "auto", "Transport type: stdio, http, or auto (uses config)")
	listen := flag.String("listen", "", "HTTP listen address (overrides config)")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s (commit: %s)\nCopyright (c) 2025, 2026 Sir Tobi aka Cybersorcerer\n", version, commit)
		os.Exit(0)
	}

	logger := log.New(os.Stderr, "[zosmf-mcp] ", log.LstdFlags|log.Lmsgprefix)

	resolvedConfig := *configPath
	if resolvedConfig == "" {
		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatalf("cannot determine working directory: %v", err)
		}
		home, _ := os.UserHomeDir()
		path, deprecated, err := config.ResolveDefaultPath(cwd, home)
		if err != nil {
			logger.Fatalf("%v", err)
		}
		if deprecated {
			logger.Printf("WARNING: %s is deprecated — rename to %s", config.LegacyConfigName, config.DefaultConfigName)
		} else {
			legacyPath := filepath.Join(cwd, config.LegacyConfigName)
			if legacyPath != path {
				if _, statErr := os.Stat(legacyPath); statErr == nil {
					logger.Printf("NOTE: using %s; ignoring legacy %s in %s", path, config.LegacyConfigName, cwd)
				}
			}
		}
		resolvedConfig = path
	}

	// Load configuration
	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		logger.Fatalf("failed to load config: %v", err)
	}

	// Create z/OSMF clients for each configured system
	clients := zosmf.NewClientRegistry()
	for _, sys := range cfg.Systems {
		client, err := zosmf.NewClient(sys)
		if err != nil {
			logger.Fatalf("failed to create client for system %s: %v", sys.Name, err)
		}
		clients.Register(sys.Name, client)
		logger.Printf("registered system: %s (%s)", sys.Name, sys.Host)
		if sys.Auth.Type == "" {
			logger.Printf("WARNING: system %s has no user configured — tool calls will fail until 'user:' is added", sys.Name)
		}
	}

	// Build SSH client registry if ssh_systems are configured
	var sshReg *zosmf.SSHClientRegistry
	if len(cfg.SSHSystems) > 0 {
		sshReg = zosmf.NewSSHClientRegistry(cfg.SSHSystems)
		logger.Printf("registered %d SSH systems", len(cfg.SSHSystems))
	}

	// Create tool registry and register all tools
	registry := mcp.NewToolRegistry()
	if err := tools.RegisterAll(registry, clients, cfg, sshReg); err != nil {
		logger.Fatalf("failed to register tools: %v", err)
	}
	logger.Printf("registered %d tools", len(registry.List()))

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Printf("received signal: %v, shutting down", sig)
		cancel()
	}()

	// Determine which transports to start:
	// --transport flag overrides config; "auto" (default) uses config.
	useStdio := cfg.Transport.Stdio
	useHTTP := cfg.Transport.HTTP.Enabled
	useStreamableHTTP := cfg.Transport.StreamableHTTP.Enabled

	switch *transport {
	case "stdio":
		useStdio = true
		useHTTP = false
		useStreamableHTTP = false
	case "http":
		useStdio = false
		useHTTP = true
		useStreamableHTTP = false
	case "streamable-http":
		useStdio = false
		useHTTP = false
		useStreamableHTTP = true
	case "auto":
		// use config values (already set above)
	default:
		fmt.Fprintf(os.Stderr, "unknown transport: %s (use 'stdio', 'http', 'streamable-http', or 'auto')\n", *transport)
		os.Exit(1)
	}

	if !useStdio && !useHTTP && !useStreamableHTTP {
		fmt.Fprintf(os.Stderr, "no transport enabled — set stdio: true, http.enabled: true, or streamable_http.enabled: true in config\n")
		os.Exit(1)
	}

	if err := cfg.ValidateHTTPAuth(useHTTP, useStreamableHTTP); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	errCh := make(chan error, 3)

	if useStdio {
		server := mcp.NewServer(registry, logger)
		t := mcp.NewStdioTransport(logger)
		logger.Println("starting stdio transport")
		go func() {
			if err := t.Run(ctx, server.HandleMessage); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("stdio: %w", err)
			} else {
				errCh <- nil
			}
		}()
	}

	if useHTTP {
		server := mcp.NewHTTPServer(registry, logger)
		addr := cfg.Transport.HTTP.Listen
		if *listen != "" {
			addr = *listen
		}
		if addr == "" {
			addr = ":8080"
		}
		t := mcp.NewHTTPTransport(addr, logger, cfg.Transport.HTTP.APIKey)
		logger.Printf("starting HTTP transport on %s", addr)
		if cfg.Transport.HTTP.APIKey == "" {
			logger.Println("WARNING: HTTP transport running without authentication")
		}
		go func() {
			if err := t.Run(ctx, server.HandleMessage); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("http: %w", err)
			} else {
				errCh <- nil
			}
		}()
	}

	if useStreamableHTTP {
		server := mcp.NewHTTPServer(registry, logger)
		addr := cfg.Transport.StreamableHTTP.Listen
		if *listen != "" {
			addr = *listen
		}
		if addr == "" {
			addr = ":8081"
		}
		t := mcp.NewHTTPTransport(addr, logger, cfg.Transport.StreamableHTTP.APIKey)
		logger.Printf("starting Streamable HTTP transport on %s", addr)
		if cfg.Transport.StreamableHTTP.APIKey == "" {
			logger.Println("WARNING: Streamable HTTP transport running without authentication")
		}
		go func() {
			if err := t.Run(ctx, server.HandleMessage); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("streamable-http: %w", err)
			} else {
				errCh <- nil
			}
		}()
	}

	// Wait for first fatal error
	if err := <-errCh; err != nil {
		logger.Fatalf("transport error: %v", err)
	}

	logger.Println("server stopped")
}
