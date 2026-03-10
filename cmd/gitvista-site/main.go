// Package main is the entry point for the hosted GitVista site.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	app "github.com/rybkr/gitvista/internal/app/site"
	"github.com/rybkr/gitvista/internal/repomanager"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

type appFlags struct {
	dataDir string
	port    string
	host    string
}

func main() {
	initLogger()

	flags, err := parseFlags(os.Args[1:])
	if err != nil {
		os.Exit(2)
	}

	portNum, _ := strconv.Atoi(flags.port)
	if portNum < 1 || portNum > 65535 {
		slog.Error("Invalid port", "port", flags.port)
		os.Exit(1)
	}

	allowedHosts := parseAllowedHosts(os.Getenv("GITVISTA_CLONE_ALLOWED_HOSTS"))
	rm, err := repomanager.New(repomanager.Config{DataDir: flags.dataDir, AllowedHosts: allowedHosts})
	if err != nil {
		slog.Error("Failed to create repo manager", "err", err)
		os.Exit(1)
	}
	if err := rm.Start(); err != nil {
		slog.Error("Failed to start repo manager", "err", err)
		os.Exit(1)
	}
	defer rm.Close()

	addr := fmt.Sprintf("%s:%s", flags.host, flags.port)
	corsOrigins := parseCORSOrigins(os.Getenv("GITVISTA_CORS_ORIGINS"))
	serv, err := app.NewServer(rm, addr, corsOrigins)
	if err != nil {
		slog.Error("Failed to load hosted frontend", "err", err)
		os.Exit(1)
	}

	slog.Info("Starting GitVista site", "version", version, "commit", commit, "built", buildDate)
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
		slog.Info("Shutdown initiated")
		stop()
		serv.Shutdown()
	}
}

func parseFlags(args []string) (appFlags, error) {
	flags := appFlags{}
	fs := flag.NewFlagSet("gitvista-site", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&flags.dataDir, "data-dir", getEnv("GITVISTA_DATA_DIR", "/data/repos"), "Data directory for managed repos")
	fs.StringVar(&flags.port, "port", getEnv("GITVISTA_PORT", "8080"), "Port to listen on")
	fs.StringVar(&flags.host, "host", getEnv("GITVISTA_HOST", ""), "Host to bind to")
	return flags, fs.Parse(args)
}

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

func parseCORSOrigins(raw string) map[string]bool {
	origins := make(map[string]bool)
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins[o] = true
		}
	}
	return origins
}

func parseAllowedHosts(raw string) []string {
	if raw == "" {
		return nil
	}
	var hosts []string
	for _, h := range strings.Split(raw, ",") {
		h = strings.TrimSpace(strings.ToLower(h))
		if h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
