// Package main is the entry point for the GitVista server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/server"
)

const version = "1.0.0-dev"

func main() {
	// Configure structured logging before anything else so that all subsequent
	// log calls — including repository loading errors — use the chosen format.
	initLogger()

	// CLI flags
	repoPath := flag.String("repo", getEnv("GITVISTA_REPO", "."), "Path to git repository")
	port := flag.String("port", getEnv("GITVISTA_PORT", "8080"), "Port to listen on")
	host := flag.String("host", getEnv("GITVISTA_HOST", ""), "Host to bind to (empty = all interfaces)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	showHelp := flag.Bool("help", false, "Show help and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("GitVista %s\n", version)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Load repository
	repo, err := gitcore.NewRepository(*repoPath)
	if err != nil {
		// log.Fatalf is used here because slog has no Fatal equivalent; the
		// standard library log package writes to stderr and calls os.Exit(1).
		log.Fatalf("Failed to load repository at %s: %v", *repoPath, err)
	}

	// Get embedded web filesystem
	webFS, err := gitvista.GetWebFS()
	if err != nil {
		log.Fatalf("Failed to load web assets: %v", err)
	}

	// Create and start server
	addr := fmt.Sprintf("%s:%s", *host, *port)
	serv := server.NewServer(repo, addr, webFS)

	slog.Info("Starting GitVista", "version", version)
	slog.Info("Repository loaded", "path", *repoPath)
	slog.Info("Listening", "addr", "http://"+addr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serv.Start()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("Server error: %v", err)
		}
	case <-ctx.Done():
		stop() // Reset signal handling so a second signal force-exits.
		serv.Shutdown()
	}
}

// initLogger reads GITVISTA_LOG_LEVEL and GITVISTA_LOG_FORMAT from the
// environment, constructs the appropriate slog.Handler, and installs it as the
// default logger via slog.SetDefault. All server-package code obtains a logger
// from slog.Default() so this single call propagates everywhere.
func initLogger() {
	// Determine log level; default to Info.
	level := slog.LevelInfo
	switch getEnv("GITVISTA_LOG_LEVEL", "info") {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{Level: level}

	// Determine output format; default to text (human-readable).
	var handler slog.Handler
	if getEnv("GITVISTA_LOG_FORMAT", "text") == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func printHelp() {
	fmt.Println("GitVista - Real-time Git repository visualization")
	fmt.Printf("Version: %s\n\n", version)
	fmt.Println("Usage:")
	fmt.Println("  vista [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -repo string")
	fmt.Println("        Path to git repository (default: current directory)")
	fmt.Println("        Environment: GITVISTA_REPO")
	fmt.Println()
	fmt.Println("  -port string")
	fmt.Println("        Port to listen on (default: 8080)")
	fmt.Println("        Environment: GITVISTA_PORT")
	fmt.Println()
	fmt.Println("  -host string")
	fmt.Println("        Host to bind to (default: all interfaces)")
	fmt.Println("        Environment: GITVISTA_HOST")
	fmt.Println()
	fmt.Println("  -version")
	fmt.Println("        Show version and exit")
	fmt.Println()
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  vista")
	fmt.Println("  vista -repo /path/to/repo")
	fmt.Println("  vista -port 3000")
	fmt.Println("  vista -host localhost -port 9090")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  GITVISTA_REPO         Default repository path")
	fmt.Println("  GITVISTA_PORT         Default port")
	fmt.Println("  GITVISTA_HOST         Default host")
	fmt.Println("  GITVISTA_LOG_LEVEL    Log level: debug, info, warn, error (default: info)")
	fmt.Println("  GITVISTA_LOG_FORMAT   Log format: text, json (default: text)")
}
