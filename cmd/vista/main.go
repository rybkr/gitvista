// Package main is the entry point for the local GitVista server.
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
	"runtime"
	"strconv"
	"syscall"
	"time"

	app "github.com/rybkr/gitvista/internal/app/local"
	"github.com/rybkr/gitvista/internal/cli"
	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/selfupdate"
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
	applyInvocationDefaults(&parsed)

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
	if err := validateConfig(parsed.repoPath, parsed.outputFormat, portNum); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", cw.Red("error:"), err) // #nosec G705
		os.Exit(1)
	}

	switch {
	case parsed.showVersion:
		printVersion(cw)
		os.Exit(0)
	case parsed.checkUpdate:
		runCheckUpdate(cw)
		os.Exit(0)
	case parsed.showHelp:
		printHelp(cw)
		os.Exit(0)
	}

	spin := cli.NewSpinner("Loading repository...")
	spin.Start()
	repoLoadStart := time.Now()
	repo, err := gitcore.NewRepository(parsed.repoPath)
	repoLoadDur := time.Since(repoLoadStart).Round(time.Millisecond)
	spin.Stop()
	if err != nil {
		slog.Error("Failed to load repository", "path", parsed.repoPath, "err", err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", resolveBindHost(parsed.host), parsed.port)
	serv, err := app.NewServer(repo, addr)
	if err != nil {
		slog.Error("Failed to load local frontend", "err", err)
		os.Exit(1)
	}

	slog.Info("Starting GitVista", "version", version, "mode", modeLocal)
	slog.Info("Repository loaded", "path", parsed.repoPath)
	slog.Info("Listening", "addr", "http://"+addr)

	if parsed.outputFormat == outputFormatJS {
		printStartupJSON(addr, parsed.repoPath, repoLoadDur)
	} else {
		printStartupBanner(cw, addr, parsed.repoPath, repoLoadDur)
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
	}
}

func parseFlags(args []string, getenv func(string, string) string) (appFlags, error) {
	flags := appFlags{}
	fs := flag.NewFlagSet("gitvista", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&flags.repoPath, "repo", getenv("GITVISTA_REPO", ""), "Path to git repository")
	fs.StringVar(&flags.port, "port", getenv("GITVISTA_PORT", "8080"), "Port to listen on")
	fs.StringVar(&flags.host, "host", getenv("GITVISTA_HOST", ""), "Host to bind to (empty = loopback)")
	fs.StringVar(&flags.color, "color", "auto", "Color output: auto, always, never")
	fs.BoolVar(&flags.noColor, "no-color", false, "Disable color output")
	fs.BoolVar(&flags.showVersion, "version", false, "Show version and exit")
	fs.BoolVar(&flags.checkUpdate, "check-update", false, "Check for a newer release and exit")
	fs.BoolVar(&flags.showHelp, "help", false, "Show help and exit")
	fs.StringVar(&flags.outputFormat, "output", "", "Startup output format: json (default: human-readable)")
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

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func applyInvocationDefaults(flags *appFlags) {
	if flags.repoPath == "" {
		flags.repoPath = "."
	}
}

func resolveBindHost(host string) string {
	if host != "" {
		return host
	}
	return "127.0.0.1"
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

func validateConfig(repoPath, outputFormat string, portNum int) error {
	if repoPath == "" {
		return fmt.Errorf("repository path is required")
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if outputFormat != "" && outputFormat != outputFormatJS {
		return fmt.Errorf("-output %q is not valid; only \"json\" is supported", outputFormat)
	}
	return nil
}

func printStartupBanner(cw *cli.Writer, addr, repoPath string, repoLoadDur time.Duration) {
	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Printf("  %s    %s\n", cw.Cyan("mode:"), modeLocal)
	timing := fmt.Sprintf("(loaded in %s)", cw.Yellow(repoLoadDur.String()))
	fmt.Printf("  %s    %s  %s\n", cw.Cyan("repo:"), repoPath, timing)
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
	RepoLoadMs int64  `json:"repo_load_ms,omitempty"`
}

func printStartupJSON(addr, repoPath string, repoLoadDur time.Duration) {
	info := startupInfo{
		Version:    version,
		Commit:     commit,
		BuildDate:  buildDate,
		Mode:       modeLocal,
		Listen:     "http://" + addr,
		RepoPath:   repoPath,
		RepoLoadMs: repoLoadDur.Milliseconds(),
	}
	data, _ := json.Marshal(info)
	fmt.Println(string(data))
}

func printHelp(cw *cli.Writer) {
	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Println("Real-time local Git repository visualization")
	fmt.Println()
	fmt.Println(cw.Bold("Usage:"))
	fmt.Println("  gitvista [flags]")
	fmt.Println()
	fmt.Println(cw.Bold("Flags:"))
	fmt.Printf("  %s string\n", cw.Flag("-repo"))
	fmt.Println("        Path to git repository (default: current directory)")
	fmt.Println("        Environment: GITVISTA_REPO")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Flag("-port, --port"))
	fmt.Println("        Port to listen on (default: 8080; long form accepted)")
	fmt.Println("        Environment: GITVISTA_PORT")
	fmt.Println()
	fmt.Printf("  %s string\n", cw.Flag("-host"))
	fmt.Println("        Host to bind to (default: 127.0.0.1)")
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
	fmt.Println("  gitvista")
	fmt.Println("  gitvista -repo /path/to/repo")
	fmt.Println("  gitvista --port 3000")
	fmt.Println("  git vista")
	fmt.Println()
	fmt.Println(cw.Bold("Environment Variables:"))
	fmt.Println("  GITVISTA_REPO         Repository path (default: current directory)")
	fmt.Println("  GITVISTA_PORT         Default port")
	fmt.Println("  GITVISTA_HOST         Default host")
	fmt.Println("  GITVISTA_LOG_LEVEL    Log level: debug, info, warn, error (default: info)")
	fmt.Println("  GITVISTA_LOG_FORMAT   Log format: text, json (default: text)")
}
