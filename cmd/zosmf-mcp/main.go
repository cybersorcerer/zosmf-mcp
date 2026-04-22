package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	configPath := flag.String("config", ".zosmf-mcp-config.yaml", "Path to configuration file")
	transport := flag.String("transport", "auto", "Transport type: stdio, http, or auto (uses config)")
	listen := flag.String("listen", "", "HTTP listen address (overrides config)")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s (commit: %s)\nCopyright (c) 2025, 2026 Sir Tobi aka Cybersorcerer\n", version, commit)
		os.Exit(0)
	}

	logger := log.New(os.Stderr, "[zosmf-mcp] ", log.LstdFlags|log.Lmsgprefix)

	// Load configuration
	cfg, err := config.Load(*configPath)
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

	// Create MCP server
	server := mcp.NewServer(registry, logger)

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

	switch *transport {
	case "stdio":
		useStdio = true
		useHTTP = false
	case "http":
		useStdio = false
		useHTTP = true
	case "auto":
		// use config values (already set above)
	default:
		fmt.Fprintf(os.Stderr, "unknown transport: %s (use 'stdio', 'http', or 'auto')\n", *transport)
		os.Exit(1)
	}

	if !useStdio && !useHTTP {
		fmt.Fprintf(os.Stderr, "no transport enabled — set stdio: true or http.enabled: true in config\n")
		os.Exit(1)
	}

	errCh := make(chan error, 2)

	if useStdio {
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
		addr := cfg.Transport.HTTP.Listen
		if *listen != "" {
			addr = *listen
		}
		if addr == "" {
			addr = ":8080"
		}
		t := mcp.NewHTTPTransport(addr, logger)
		logger.Printf("starting HTTP transport on %s", addr)
		go func() {
			if err := t.Run(ctx, server.HandleMessage); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("http: %w", err)
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
