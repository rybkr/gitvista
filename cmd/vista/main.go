// Package main is the entry point for the GitVista server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/internal/cli"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/repomanager"
	"github.com/rybkr/gitvista/internal/selfupdate"
	"github.com/rybkr/gitvista/internal/server"
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

type appFlags struct {
	repoPath     string
	dataDir      string
	port         string
	host         string
	color        string
	noColor      bool
	showVersion  bool
	checkUpdate  bool
	showHelp     bool
	outputFormat string
}

func main() {
	initLogger()

	parsed, err := parseFlags(os.Args[1:], getEnv)
	if err != nil {
		os.Exit(2)
	}
	applyInvocationDefaults(&parsed, os.Args[0])

	// Resolve color mode.
	colorMode := cli.ColorAuto
	if parsed.noColor {
		colorMode = cli.ColorNever
	} else if parsed.color != "auto" {
		colorMode, err = cli.ParseColorMode(parsed.color)
		if err != nil {
			slog.Error("Invalid color flag", "value", parsed.color, "err", err)
			os.Exit(1)
		}
	}
	cw := cli.NewWriter(os.Stdout, colorMode)

	portNum, _ := strconv.Atoi(parsed.port)
	if err := validateConfig(parsed.repoPath, parsed.dataDir, parsed.outputFormat, portNum); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", cw.Red("error:"), err) // #nosec G705 -- error message to stderr, not HTTP response
		os.Exit(1)
	}

	if parsed.showVersion {
		printVersion(cw)
		os.Exit(0)
	}

	if parsed.checkUpdate {
		runCheckUpdate(cw)
		os.Exit(0)
	}

	if parsed.showHelp {
		printHelp(cw)
		os.Exit(0)
	}

	// Get embedded web filesystem
	webFS, err := gitvista.GetWebFS()
	if err != nil {
		slog.Error("Failed to load web assets", "err", err)
		os.Exit(1)
	}

	bindHost := resolveBindHost(parsed.repoPath, parsed.host)
	addr := fmt.Sprintf("%s:%s", bindHost, parsed.port)

	var serv interface {
		Start() error
		Shutdown()
	}

	var rm *repomanager.RepoManager
	var repoLoadDur time.Duration

	if parsed.repoPath != "" {
		// LOCAL MODE: load repo, create local server
		spin := cli.NewSpinner("Loading repository...")
		spin.Start()
		repoLoadStart := time.Now()
		repo, err := gitcore.NewRepository(parsed.repoPath)
		repoLoadDur = time.Since(repoLoadStart).Round(time.Millisecond)
		spin.Stop()
		if err != nil {
			slog.Error("Failed to load repository", "path", parsed.repoPath, "err", err)
			os.Exit(1)
		}

		serv = server.NewLocalServer(repo, addr, webFS)

		slog.Info("Starting GitVista", "version", version, "mode", modeLocal)
		slog.Info("Repository loaded", "path", parsed.repoPath)
	} else {
		// HOSTED MODE: create RepoManager, start it, create hosted server
		var err error
		allowedHosts := parseAllowedHosts(os.Getenv("GITVISTA_CLONE_ALLOWED_HOSTS"))
		rm, err = repomanager.New(repomanager.Config{DataDir: parsed.dataDir, AllowedHosts: allowedHosts})
		if err != nil {
			slog.Error("Failed to create repo manager", "err", err)
			os.Exit(1)
		}

		if err := rm.Start(); err != nil {
			slog.Error("Failed to start repo manager", "err", err)
			os.Exit(1)
		}

		corsOrigins := parseCORSOrigins(os.Getenv("GITVISTA_CORS_ORIGINS"))
		serv = server.NewHostedServer(rm, addr, webFS, corsOrigins)

		slog.Info("Starting GitVista", "version", version, "mode", "hosted")
		slog.Info("Data directory", "path", parsed.dataDir)
	}

	slog.Info("Listening", "addr", "http://"+addr)

	mode := "hosted"
	if parsed.repoPath != "" {
		mode = modeLocal
	}
	if parsed.outputFormat == outputFormatJS {
		printStartupJSON(mode, addr, parsed.repoPath, parsed.dataDir, repoLoadDur)
	} else {
		printStartupBanner(cw, mode, addr, parsed.repoPath, parsed.dataDir, repoLoadDur)
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

func parseFlags(args []string, getenv func(string, string) string) (appFlags, error) {
	flags := appFlags{}
	fs := flag.NewFlagSet("gitvista", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&flags.repoPath, "repo", getenv("GITVISTA_REPO", ""), "Path to git repository (local mode)")
	fs.StringVar(&flags.dataDir, "data-dir", getenv("GITVISTA_DATA_DIR", "/data/repos"), "Data directory for managed repos (hosted mode)")
	fs.StringVar(&flags.port, "port", getenv("GITVISTA_PORT", "8080"), "Port to listen on")
	fs.StringVar(&flags.host, "host", getenv("GITVISTA_HOST", ""), "Host to bind to (empty = all interfaces)")
	fs.StringVar(&flags.color, "color", "auto", "Color output: auto, always, never")
	fs.BoolVar(&flags.noColor, "no-color", false, "Disable color output")
	fs.BoolVar(&flags.showVersion, "version", false, "Show version and exit")
	fs.BoolVar(&flags.checkUpdate, "check-update", false, "Check for a newer release and exit")
	fs.BoolVar(&flags.showHelp, "help", false, "Show help and exit")
	fs.StringVar(&flags.outputFormat, "output", "", "Startup output format: json (default: human-readable)")
	return flags, fs.Parse(args)
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

// parseCORSOrigins splits a comma-separated list of origins into a lookup map.
// Returns an empty (non-nil) map if the input is empty, which means no cross-origin
// requests will be allowed (same-origin only).
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

// parseAllowedHosts splits a comma-separated list of hostnames into a slice.
// Returns nil if the input is empty, which causes the RepoManager to use its
// default allowlist (github.com, gitlab.com, bitbucket.org).
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

func applyInvocationDefaults(flags *appFlags, argv0 string) {
	if flags.repoPath != "" {
		return
	}
	if filepath.Base(argv0) == "git-vista" {
		flags.repoPath = "."
	}
}

// resolveBindHost chooses a safe default bind host.
// In local mode (repoPath set), default to loopback when host is empty.
// In hosted mode, preserve the existing empty-host behavior (all interfaces).
func resolveBindHost(repoPath, host string) string {
	if host != "" {
		return host
	}
	if repoPath != "" {
		return "127.0.0.1"
	}
	return ""
}

func printVersion(cw *cli.Writer) {
	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Printf("  %s  %s\n", cw.Cyan("commit:"), commit)
	fmt.Printf("  %s   %s\n", cw.Cyan("built:"), buildDate)
	fmt.Printf("  %s %s\n", cw.Cyan("go version:"), runtime.Version())
	fmt.Printf("  %s %s/%s\n", cw.Cyan("platform:"), runtime.GOOS, runtime.GOARCH)
}

func runCheckUpdate(cw *cli.Writer) {
	const repo = "rybkr/gitvista"
	fmt.Printf("%s %s\n", cw.Cyan("Current version:"), version)

	latest, err := selfupdate.CheckLatest(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", cw.Red("Error checking for updates:"), err)
		os.Exit(1)
	}
	fmt.Printf("%s  %s\n", cw.Cyan("Latest version:"), latest)

	if !selfupdate.NeedsUpdate(version, latest) {
		if version == "dev" {
			fmt.Println("Development build — skipping update check.")
		} else {
			fmt.Println("Already up to date.")
		}
		return
	}

	fmt.Printf("\n%s %s → %s\n", cw.Bold("Update available:"), version, cw.Green(latest))
	fmt.Println("To update, run one of:")
	fmt.Printf("  %s\n", cw.Command("gitvista-cli update"))
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

func printStartupBanner(cw *cli.Writer, mode, addr, repoPath, dataDir string, repoLoadDur time.Duration) {
	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Printf("  %s    %s\n", cw.Cyan("mode:"), mode)
	if mode == modeLocal {
		timing := fmt.Sprintf("(loaded in %s)", cw.Yellow(repoLoadDur.String()))
		fmt.Printf("  %s    %s  %s\n", cw.Cyan("repo:"), repoPath, timing)
	} else {
		fmt.Printf("  %s    %s\n", cw.Cyan("data:"), dataDir)
	}
	fmt.Printf("  %s  http://%s\n", cw.Cyan("listen:"), addr)
	fmt.Printf("  %s  %s\n", cw.Cyan("commit:"), commit)
	if cli.IsTerminal(os.Stdout.Fd()) {
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

func printHelp(cw *cli.Writer) {
	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Println("Real-time Git repository visualization")
	fmt.Println()
	fmt.Println(cw.Bold("Usage:"))
	fmt.Println("  gitvista [flags]")
	fmt.Println()
	fmt.Println(cw.Bold("Flags:"))
	fmt.Printf("  %s string\n", cw.Flag("-repo"))
	fmt.Println("        Path to git repository (local mode)")
	fmt.Println("        Environment: GITVISTA_REPO")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Flag("-data-dir"))
	fmt.Println("        Data directory for managed repos (hosted mode, default: /data/repos)")
	fmt.Println("        Environment: GITVISTA_DATA_DIR")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Flag("-port, --port"))
	fmt.Println("        Port to listen on (default: 8080; long form accepted)")
	fmt.Println("        Environment: GITVISTA_PORT")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Flag("-host"))
	fmt.Println("        Host to bind to (default: all interfaces)")
	fmt.Println("        Environment: GITVISTA_HOST")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Flag("-output"))
	fmt.Println("        Startup output format: json (default: human-readable)")
	fmt.Println()
	fmt.Printf("  %s\n", cw.Flag("-version"))
	fmt.Println("        Show version and exit")
	fmt.Println()
	fmt.Printf("  %s\n", cw.Flag("-check-update"))
	fmt.Println("        Check for a newer release and exit")
	fmt.Println()
	fmt.Printf("  %s\n", cw.Flag("-help"))
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println(cw.Bold("Examples:"))
	fmt.Println("  gitvista -repo .              # local mode: current directory")
	fmt.Println("  gitvista -repo /path/to/repo  # local mode: specific repo")
	fmt.Println("  git vista                     # local mode via git subcommand")
	fmt.Println("  gitvista                      # hosted mode: manage repos via API")
	fmt.Println("  gitvista --port 3000")
	fmt.Println("  gitvista -host localhost -port 9090")
	fmt.Println()
	fmt.Println(cw.Bold("Environment Variables:"))
	fmt.Println("  GITVISTA_REPO         Repository path (sets local mode)")
	fmt.Println("  GITVISTA_DATA_DIR     Data directory for hosted mode")
	fmt.Println("  GITVISTA_PORT         Default port")
	fmt.Println("  GITVISTA_HOST         Default host")
	fmt.Println("  GITVISTA_LOG_LEVEL    Log level: debug, info, warn, error (default: info)")
	fmt.Println("  GITVISTA_LOG_FORMAT   Log format: text, json (default: text)")
}
