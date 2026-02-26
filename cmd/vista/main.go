// Package main is the entry point for the GitVista server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/progress"
	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/selfupdate"
	"github.com/rybkr/gitvista/internal/server"
	"github.com/rybkr/gitvista/internal/termcolor"
)

const (
	modeLocal      = "local"
	outputFormatJS = "json"
)

// Build-time variables set via -ldflags.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	initLogger()

	// CLI flags
	repoPath := flag.String("repo", getEnv("GITVISTA_REPO", ""), "Path to git repository (local mode)")
	dataDir := flag.String("data-dir", getEnv("GITVISTA_DATA_DIR", "/data/repos"), "Data directory for managed repos (SaaS mode)")
	port := flag.String("port", getEnv("GITVISTA_PORT", "8080"), "Port to listen on")
	host := flag.String("host", getEnv("GITVISTA_HOST", ""), "Host to bind to (empty = all interfaces)")
	colorFlag := flag.String("color", "auto", "Color output: auto, always, never")
	noColor := flag.Bool("no-color", false, "Disable color output")
	showVersion := flag.Bool("version", false, "Show version and exit")
	checkUpdate := flag.Bool("check-update", false, "Check for a newer release and exit")
	showHelp := flag.Bool("help", false, "Show help and exit")
	outputFormat := flag.String("output", "", "Startup output format: json (default: human-readable)")

	flag.Parse()

	// Resolve color mode.
	colorMode := termcolor.ColorAuto
	if *noColor {
		colorMode = termcolor.ColorNever
	} else if *colorFlag != "auto" {
		var err error
		colorMode, err = termcolor.ParseColorMode(*colorFlag)
		if err != nil {
			slog.Error("Invalid color flag", "value", *colorFlag, "err", err)
			os.Exit(1)
		}
	}
	cw := termcolor.NewWriter(os.Stdout, colorMode)

	portNum, _ := strconv.Atoi(*port)
	if err := validateConfig(*repoPath, *dataDir, *outputFormat, portNum); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", cw.Red("error:"), err) // #nosec G705 -- error message to stderr, not HTTP response
		os.Exit(1)
	}

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	if *checkUpdate {
		runCheckUpdate()
		os.Exit(0)
	}

	if *showHelp {
		printHelp(cw)
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
	var repoLoadDur time.Duration

	if *repoPath != "" {
		// LOCAL MODE: load repo, create local server
		spin := progress.New("Loading repository...")
		spin.Start()
		repoLoadStart := time.Now()
		repo, err := gitcore.NewRepository(*repoPath)
		repoLoadDur = time.Since(repoLoadStart).Round(time.Millisecond)
		spin.Stop()
		if err != nil {
			slog.Error("Failed to load repository", "path", *repoPath, "err", err)
			os.Exit(1)
		}

		serv = server.NewLocalServer(repo, addr, webFS)

		slog.Info("Starting GitVista", "version", version, "mode", modeLocal)
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

	mode := "saas"
	if *repoPath != "" {
		mode = modeLocal
	}
	if *outputFormat == outputFormatJS {
		printStartupJSON(mode, addr, *repoPath, *dataDir, repoLoadDur)
	} else {
		printStartupBanner(cw, mode, addr, *repoPath, *dataDir, repoLoadDur)
	}

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

func printVersion() {
	fmt.Printf("GitVista %s\n", version)
	fmt.Printf("  commit:     %s\n", commit)
	fmt.Printf("  built:      %s\n", buildDate)
	fmt.Printf("  go version: %s\n", runtime.Version())
	fmt.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func runCheckUpdate() {
	const repo = "rybkr/gitvista"
	fmt.Printf("Current version: %s\n", version)

	latest, err := selfupdate.CheckLatest(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Latest version:  %s\n", latest)

	if !selfupdate.NeedsUpdate(version, latest) {
		if version == "dev" {
			fmt.Println("Development build — skipping update check.")
		} else {
			fmt.Println("Already up to date.")
		}
		return
	}

	fmt.Printf("\nUpdate available: %s → %s\n", version, latest)
	fmt.Println("To update, run one of:")
	fmt.Println("  gitvista-cli update")
	fmt.Println("  brew upgrade gitvista")
}

func validateConfig(repoPath, dataDir, outputFormat string, portNum int) error {
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if repoPath != "" && dataDir != "/data/repos" {
		return fmt.Errorf("-repo and -data-dir are mutually exclusive")
	}
	if outputFormat != "" && outputFormat != outputFormatJS {
		return fmt.Errorf("-output %q is not valid; only \"json\" is supported", outputFormat)
	}
	return nil
}

func printStartupBanner(cw *termcolor.Writer, mode, addr, repoPath, dataDir string, repoLoadDur time.Duration) {
	fmt.Printf("%s %s\n", cw.BoldCyan("GitVista"), cw.Green(version))
	fmt.Printf("  mode:    %s\n", mode)
	if mode == modeLocal {
		timing := fmt.Sprintf("(loaded in %s)", cw.Yellow(repoLoadDur.String()))
		fmt.Printf("  repo:    %s  %s\n", repoPath, timing)
	} else {
		fmt.Printf("  data:    %s\n", dataDir)
	}
	fmt.Printf("  listen:  http://%s\n", addr)
	fmt.Printf("  commit:  %s\n", commit)
	if termcolor.IsTerminal(os.Stdout.Fd()) {
		fmt.Printf("\n%s\n", cw.Bold("Press Ctrl+C to stop."))
	}
}

type startupInfo struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildDate  string `json:"build_date"`
	Mode       string `json:"mode"`
	Listen     string `json:"listen"`
	RepoPath   string `json:"repo_path,omitempty"`
	DataDir    string `json:"data_dir,omitempty"`
	RepoLoadMs int64  `json:"repo_load_ms,omitempty"`
}

func printStartupJSON(mode, addr, repoPath, dataDir string, repoLoadDur time.Duration) {
	info := startupInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
		Mode:      mode,
		Listen:    "http://" + addr,
	}
	if mode == modeLocal {
		info.RepoPath = repoPath
		info.RepoLoadMs = repoLoadDur.Milliseconds()
	} else {
		info.DataDir = dataDir
	}
	data, _ := json.Marshal(info)
	fmt.Println(string(data))
}

func printHelp(cw *termcolor.Writer) {
	fmt.Println("GitVista - Real-time Git repository visualization")
	fmt.Printf("Version: %s\n\n", version)
	fmt.Println(cw.Bold("Usage:"))
	fmt.Println("  gitvista [flags]")
	fmt.Println()
	fmt.Println(cw.Bold("Flags:"))
	fmt.Printf("  %s string\n", cw.Yellow("-repo"))
	fmt.Println("        Path to git repository (local mode)")
	fmt.Println("        Environment: GITVISTA_REPO")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Yellow("-data-dir"))
	fmt.Println("        Data directory for managed repos (SaaS mode, default: /data/repos)")
	fmt.Println("        Environment: GITVISTA_DATA_DIR")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Yellow("-port"))
	fmt.Println("        Port to listen on (default: 8080)")
	fmt.Println("        Environment: GITVISTA_PORT")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Yellow("-host"))
	fmt.Println("        Host to bind to (default: all interfaces)")
	fmt.Println("        Environment: GITVISTA_HOST")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Yellow("-output"))
	fmt.Println("        Startup output format: json (default: human-readable)")
	fmt.Println()
	fmt.Printf("  %s\n", cw.Yellow("-version"))
	fmt.Println("        Show version and exit")
	fmt.Println()
	fmt.Printf("  %s\n", cw.Yellow("-check-update"))
	fmt.Println("        Check for a newer release and exit")
	fmt.Println()
	fmt.Printf("  %s\n", cw.Yellow("-help"))
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println(cw.Bold("Examples:"))
	fmt.Println("  gitvista -repo .              # local mode: current directory")
	fmt.Println("  gitvista -repo /path/to/repo  # local mode: specific repo")
	fmt.Println("  gitvista                      # SaaS mode: manage repos via API")
	fmt.Println("  gitvista -port 3000")
	fmt.Println("  gitvista -host localhost -port 9090")
	fmt.Println()
	fmt.Println(cw.Bold("Environment Variables:"))
	fmt.Println("  GITVISTA_REPO         Repository path (sets local mode)")
	fmt.Println("  GITVISTA_DATA_DIR     Data directory for SaaS mode")
	fmt.Println("  GITVISTA_PORT         Default port")
	fmt.Println("  GITVISTA_HOST         Default host")
	fmt.Println("  GITVISTA_LOG_LEVEL    Log level: debug, info, warn, error (default: info)")
	fmt.Println("  GITVISTA_LOG_FORMAT   Log format: text, json (default: text)")
}
