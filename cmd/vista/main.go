// Package main is the entry point for the GitVista server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/server"
)

// version is set at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	initLogger()

	// CLI flags
	repoPath := flag.String("repo", getEnv("GITVISTA_REPO", ""), "Path to git repository (local mode)")
	dataDir := flag.String("data-dir", getEnv("GITVISTA_DATA_DIR", "/data/repos"), "Data directory for managed repos (SaaS mode)")
	port := flag.String("port", getEnv("GITVISTA_PORT", "8080"), "Port to listen on")
	host := flag.String("host", getEnv("GITVISTA_HOST", ""), "Host to bind to (empty = all interfaces)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	showHelp := flag.Bool("help", false, "Show help and exit")

	flag.Parse()

	portNum, err := strconv.Atoi(*port)
	if err != nil || portNum < 1 || portNum > 65535 {
		slog.Error("Invalid port number", "port", *port)
		os.Exit(1)
	}

	if *showVersion {
		fmt.Printf("GitVista %s\n", version)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Get embedded web filesystem
	webFS, err := gitvista.GetWebFS()
	if err != nil {
		slog.Error("Failed to load web assets", "err", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", *host, *port)

	var serv interface {
		Start() error
		Shutdown()
	}

	var rm *repomanager.RepoManager

	if *repoPath != "" {
		// LOCAL MODE: load repo, create local server
		repo, err := gitcore.NewRepository(*repoPath)
		if err != nil {
			slog.Error("Failed to load repository", "path", *repoPath, "err", err)
			os.Exit(1)
		}

		serv = server.NewLocalServer(repo, addr, webFS)

		slog.Info("Starting GitVista", "version", version, "mode", "local")
		slog.Info("Repository loaded", "path", *repoPath)
	} else {
		// SAAS MODE: create RepoManager, start it, create SaaS server
		var err error
		rm, err = repomanager.New(repomanager.Config{DataDir: *dataDir})
		if err != nil {
			slog.Error("Failed to create repo manager", "err", err)
			os.Exit(1)
		}

		if err := rm.Start(); err != nil {
			slog.Error("Failed to start repo manager", "err", err)
			os.Exit(1)
		}

		serv = server.NewSaaSServer(rm, addr, webFS)

		slog.Info("Starting GitVista", "version", version, "mode", "saas")
		slog.Info("Data directory", "path", *dataDir)
	}

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
			slog.Error("Server error", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("Shutdown initiated, press Ctrl+C again to force exit")
		stop()
		serv.Shutdown()
		if rm != nil {
			slog.Info("Stopping repo manager")
			rm.Close()
			slog.Info("Repo manager stopped")
		}
	}
}

// initLogger reads GITVISTA_LOG_LEVEL and GITVISTA_LOG_FORMAT from the
// environment, constructs the appropriate slog.Handler, and installs it as the
// default logger via slog.SetDefault.
func initLogger() {
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
	fmt.Println("  gitvista [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -repo string")
	fmt.Println("        Path to git repository (local mode)")
	fmt.Println("        Environment: GITVISTA_REPO")
	fmt.Println()
	fmt.Println("  -data-dir string")
	fmt.Println("        Data directory for managed repos (SaaS mode, default: /data/repos)")
	fmt.Println("        Environment: GITVISTA_DATA_DIR")
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
	fmt.Println("  gitvista -repo .              # local mode: current directory")
	fmt.Println("  gitvista -repo /path/to/repo  # local mode: specific repo")
	fmt.Println("  gitvista                      # SaaS mode: manage repos via API")
	fmt.Println("  gitvista -port 3000")
	fmt.Println("  gitvista -host localhost -port 9090")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  GITVISTA_REPO         Repository path (sets local mode)")
	fmt.Println("  GITVISTA_DATA_DIR     Data directory for SaaS mode")
	fmt.Println("  GITVISTA_PORT         Default port")
	fmt.Println("  GITVISTA_HOST         Default host")
	fmt.Println("  GITVISTA_LOG_LEVEL    Log level: debug, info, warn, error (default: info)")
	fmt.Println("  GITVISTA_LOG_FORMAT   Log format: text, json (default: text)")
}
