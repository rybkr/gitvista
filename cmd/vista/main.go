// Package main is the entry point for the GitVista server.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	neturl "net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rybkr/gitvista"
	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/cli"
	"github.com/rybkr/gitvista/internal/selfupdate"
	"github.com/rybkr/gitvista/internal/server"
)

const (
	outputFormatJS = "json"
)

const (
	commandHelp   = "help"
	commandOpen   = "open"
	commandServe  = "serve"
	commandURL    = "url"
	commandDoctor = "doctor"
	commandUpdate = "update"
)

// Build-time variables set via -ldflags.
var (
	version             = "dev"
	commit              = "unknown"
	buildDate           = "unknown"
	checkLatestFunc     = selfupdate.CheckLatest
	performUpdateFunc   = selfupdate.Update
	resolveExecPathFunc = resolveExecutablePath
)

type appFlags struct {
	command      string
	repoPath     string
	port         string
	host         string
	color        string
	noColor      bool
	showVersion  bool
	checkUpdate  bool
	showHelp     bool
	outputFormat string
	noBrowser    bool
	printURL     bool
	branch       string
	targetRev    string
	targetPath   string
	jsonOutput   bool
}

type launchTarget struct {
	CommitHash gitcore.Hash
	Path       string
}

type startupInfo struct {
	Version    string `json:"version"`
	Commit     string `json:"commit"`
	BuildDate  string `json:"build_date"`
	Listen     string `json:"listen"`
	OpenURL    string `json:"open_url,omitempty"`
	RepoPath   string `json:"repo_path,omitempty"`
	RepoLoadMs int64  `json:"repo_load_ms,omitempty"`
	Command    string `json:"command,omitempty"`
}

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type doctorReport struct {
	OK      bool          `json:"ok"`
	Command string        `json:"command"`
	Checks  []doctorCheck `json:"checks"`
}

func main() {
	initLogger()

	parsed, err := parseFlags(os.Args[1:], getEnv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
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

	if parsed.command == "" {
		parsed.command = commandServe
	}

	switch {
	case parsed.showVersion:
		printVersion(cw)
		os.Exit(0)
	case parsed.checkUpdate:
		runCheckUpdate(cw)
		os.Exit(0)
	case parsed.showHelp:
		printHelp(cw, parsed.command)
		os.Exit(0)
	}

	portNum, _ := strconv.Atoi(parsed.port)
	if parsed.command == commandOpen || parsed.command == commandServe || parsed.command == commandDoctor || parsed.command == commandURL {
		if err := validateConfig(parsed.repoPath, parsed.outputFormat, portNum); err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", cw.Red("error:"), err) // #nosec G705
			os.Exit(1)
		}
	}

	switch parsed.command {
	case commandServe:
		os.Exit(runServe(parsed, cw, false))
	case commandOpen:
		os.Exit(runServe(parsed, cw, true))
	case commandURL:
		os.Exit(runURL(parsed))
	case commandDoctor:
		os.Exit(runDoctor(parsed, cw))
	case commandUpdate:
		os.Exit(runUpdate(cw))
	default:
		fmt.Fprintf(os.Stderr, "%s unknown command %q\n", cw.Red("error:"), parsed.command)
		os.Exit(1)
	}
}

func parseFlags(args []string, getenv func(string, string) string) (appFlags, error) {
	flags := appFlags{
		command: commandServe,
		color:   "auto",
	}
	if len(args) > 0 {
		switch args[0] {
		case commandOpen, commandServe, commandURL, commandDoctor, commandUpdate:
			flags.command = args[0]
			args = args[1:]
		case commandHelp:
			flags.command = commandHelp
			flags.showHelp = true
			if len(args) > 1 {
				switch args[1] {
				case commandHelp, commandOpen, commandServe, commandURL, commandDoctor, commandUpdate:
					flags.command = args[1]
				}
			}
			return flags, nil
		}
	}

	var parseOutput bytes.Buffer
	fs := flag.NewFlagSet("gitvista "+flags.command, flag.ContinueOnError)
	fs.SetOutput(&parseOutput)
	fs.StringVar(&flags.repoPath, "repo", getenv("GITVISTA_REPO", ""), "Path to git repository")
	fs.StringVar(&flags.port, "port", getenv("GITVISTA_PORT", "8080"), "Port to listen on")
	fs.StringVar(&flags.host, "host", getenv("GITVISTA_HOST", ""), "Host to bind to (empty = loopback)")
	fs.StringVar(&flags.color, "color", "auto", "Color output: auto, always, never")
	fs.BoolVar(&flags.noColor, "no-color", false, "Disable color output")
	fs.BoolVar(&flags.showVersion, "version", false, "Show version and exit")
	fs.BoolVar(&flags.checkUpdate, "check-update", false, "Check for a newer release and exit")
	fs.BoolVar(&flags.showHelp, "help", false, "Show help and exit")
	fs.BoolVar(&flags.showHelp, "h", false, "Show help and exit")

	switch flags.command {
	case commandOpen:
		fs.StringVar(&flags.outputFormat, "output", "", "Startup output format: json")
		fs.BoolVar(&flags.noBrowser, "no-browser", false, "Start GitVista without launching a browser")
		fs.BoolVar(&flags.printURL, "print-url", false, "Print the resolved launch URL")
		fs.StringVar(&flags.branch, "branch", "", "Open the graph focused on a branch tip")
		fs.StringVar(&flags.targetRev, "commit", "", "Open the graph focused on a commit or revision")
		fs.StringVar(&flags.targetPath, "path", "", "Open the file explorer focused on a path")
	case commandServe:
		fs.StringVar(&flags.outputFormat, "output", "", "Startup output format: json")
	case commandURL:
		fs.StringVar(&flags.branch, "branch", "", "Build a URL focused on a branch tip")
		fs.StringVar(&flags.targetRev, "commit", "", "Build a URL focused on a commit or revision")
		fs.StringVar(&flags.targetPath, "path", "", "Build a URL focused on a path")
		fs.BoolVar(&flags.jsonOutput, "json", false, "Print structured JSON output")
	case commandDoctor:
		fs.BoolVar(&flags.jsonOutput, "json", false, "Print structured JSON output")
	}

	if err := fs.Parse(args); err != nil {
		msg := strings.TrimSpace(parseOutput.String())
		if msg == "" {
			msg = err.Error()
		}
		return flags, fmt.Errorf("%s", msg)
	}

	rest := fs.Args()
	if len(rest) == 0 {
		return flags, nil
	}

	if flags.command == commandOpen && len(rest) == 1 && flags.targetRev == "" {
		flags.targetRev = rest[0]
		return flags, nil
	}

	return flags, fmt.Errorf("unexpected argument: %s", rest[0])
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

	latest, err := checkLatestFunc(repo)
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
	fmt.Printf("  %s\n", cw.Command("gitvista update"))
	if installHint := updateInstructionForExecutable(); installHint != "" {
		fmt.Printf("  %s\n", installHint)
	}
}

func runUpdate(cw *cli.Writer) int {
	const (
		repo    = "rybkr/gitvista"
		project = "gitvista"
	)

	if version == "dev" || version == "" {
		fmt.Fprintf(os.Stderr, "%s self-update is unavailable for development builds\n", cw.Red("error:"))
		return 1
	}

	execPath, err := resolveExecPathFunc()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s resolve executable: %v\n", cw.Red("error:"), err)
		return 1
	}

	if hint := updateInstructionForPath(execPath); hint != "" {
		fmt.Fprintf(os.Stderr, "%s this installation is managed externally; run %s\n", cw.Red("error:"), cw.Command(hint))
		return 1
	}

	latest, err := checkLatestFunc(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s check latest version: %v\n", cw.Red("error:"), err)
		return 1
	}

	fmt.Printf("%s %s\n", cw.Cyan("Current version:"), version)
	fmt.Printf("%s  %s\n", cw.Cyan("Latest version:"), latest)

	if !selfupdate.NeedsUpdate(version, latest) {
		fmt.Println("Already up to date.")
		return 0
	}

	fmt.Printf("%s %s → %s\n", cw.Bold("Updating:"), version, cw.Green(latest))
	if err := performUpdateFunc(repo, project, latest); err != nil {
		fmt.Fprintf(os.Stderr, "%s update failed: %v\n", cw.Red("error:"), err)
		return 1
	}

	fmt.Printf("%s Updated GitVista to %s\n", cw.Green("success:"), latest)
	return 0
}

func resolveExecutablePath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(execPath)
}

func updateInstructionForExecutable() string {
	execPath, err := resolveExecPathFunc()
	if err != nil {
		return ""
	}
	return updateInstructionForPath(execPath)
}

func updateInstructionForPath(execPath string) string {
	cleanPath := filepath.Clean(execPath)
	needle := string(filepath.Separator) + filepath.Join("Cellar", "gitvista") + string(filepath.Separator)
	if strings.Contains(cleanPath, needle) {
		return "brew upgrade gitvista"
	}
	return ""
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

func runServe(parsed appFlags, cw *cli.Writer, launchBrowser bool) int {
	spin := cli.NewSpinner("Loading repository...")
	spin.Start()
	repoLoadStart := time.Now()
	repo, err := gitcore.NewRepository(parsed.repoPath)
	repoLoadDur := time.Since(repoLoadStart).Round(time.Millisecond)
	spin.Stop()
	if err != nil {
		slog.Error("Failed to load repository", "path", parsed.repoPath, "err", err)
		return 1
	}
	repoOwned := true
	defer func() {
		if !repoOwned {
			return
		}
		if err := repo.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "%s close repository: %v\n", cw.Red("error:"), err)
		}
	}()

	target, err := resolveLaunchTarget(repo, parsed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", cw.Red("error:"), err)
		return 1
	}

	addr := fmt.Sprintf("%s:%s", resolveBindHost(parsed.host), parsed.port)
	baseURL, openURL := buildURLs(addr, target)

	webFS, err := gitvista.GetWebFS()
	if err != nil {
		slog.Error("Failed to load frontend assets", "err", err)
		return 1
	}

	serv := server.NewServer(repo, addr, webFS)
	repoOwned = false

	slog.Info("Starting GitVista", "version", version, "command", parsed.command)
	slog.Info("Repository loaded", "path", parsed.repoPath)
	slog.Info("Listening for GitVista requests")

	if parsed.outputFormat == outputFormatJS {
		printStartupJSON(parsed.command, baseURL, openURL, parsed.repoPath, repoLoadDur)
	} else {
		printStartupBanner(cw, parsed.command, baseURL, openURL, parsed.repoPath, repoLoadDur, launchBrowser)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serv.Start()
	}()

	if launchBrowser && !parsed.noBrowser {
		if parsed.printURL {
			fmt.Println(openURL)
		}
		go func(url string) {
			time.Sleep(150 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				slog.Warn("Failed to open browser", "err", err)
			}
		}(openURL)
	} else if parsed.printURL {
		fmt.Println(openURL)
	}

	select {
	case err := <-errCh:
		if err != nil {
			slog.Error("Server error", "err", err)
			return 1
		}
	case <-ctx.Done():
		slog.Info("Shutdown initiated, press Ctrl+C again to force exit")
		stop()
		serv.Shutdown()
	}

	return 0
}

func runURL(parsed appFlags) int {
	repo, err := gitcore.NewRepository(parsed.repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() {
		if err := repo.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "error: close repository: %v\n", err)
		}
	}()

	target, err := resolveLaunchTarget(repo, parsed)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	addr := fmt.Sprintf("%s:%s", resolveBindHost(parsed.host), parsed.port)
	baseURL, openURL := buildURLs(addr, target)

	if parsed.jsonOutput {
		data, _ := json.Marshal(struct {
			Command string `json:"command"`
			Listen  string `json:"listen"`
			OpenURL string `json:"open_url"`
		}{
			Command: commandURL,
			Listen:  baseURL,
			OpenURL: openURL,
		})
		fmt.Println(string(data))
		return 0
	}

	fmt.Println(openURL)
	return 0
}

func runDoctor(parsed appFlags, cw *cli.Writer) int {
	addr := fmt.Sprintf("%s:%s", resolveBindHost(parsed.host), parsed.port)
	report := doctorReport{
		OK:      true,
		Command: commandDoctor,
		Checks:  make([]doctorCheck, 0, 3),
	}

	repo, err := gitcore.NewRepository(parsed.repoPath)
	if err != nil {
		report.OK = false
		report.Checks = append(report.Checks, doctorCheck{
			Name:    "repository",
			Status:  "fail",
			Message: err.Error(),
		})
	} else {
		defer func() {
			if err := repo.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "%s close repository: %v\n", cw.Red("error:"), err)
			}
		}()
		report.Checks = append(report.Checks, doctorCheck{
			Name:    "repository",
			Status:  "ok",
			Message: fmt.Sprintf("loaded %s", parsed.repoPath),
		})
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		report.OK = false
		report.Checks = append(report.Checks, doctorCheck{
			Name:    "listener",
			Status:  "fail",
			Message: err.Error(),
		})
	} else {
		_ = ln.Close()
		report.Checks = append(report.Checks, doctorCheck{
			Name:    "listener",
			Status:  "ok",
			Message: fmt.Sprintf("can bind %s", addr),
		})
	}

	if launcher, err := browserLauncher(); err != nil {
		report.Checks = append(report.Checks, doctorCheck{
			Name:    "browser",
			Status:  "warn",
			Message: err.Error(),
		})
	} else {
		report.Checks = append(report.Checks, doctorCheck{
			Name:    "browser",
			Status:  "ok",
			Message: fmt.Sprintf("launcher %s", strings.Join(launcher, " ")),
		})
	}

	if parsed.jsonOutput {
		data, _ := json.Marshal(report)
		fmt.Println(string(data))
		if report.OK {
			return 0
		}
		return 1
	}

	fmt.Printf("%s %s\n", cw.Command("GitVista doctor"), cw.Muted(version))
	for _, check := range report.Checks {
		label := check.Status
		switch check.Status {
		case "ok":
			label = cw.Green("ok")
		case "warn":
			label = cw.Yellow("warn")
		case "fail":
			label = cw.Red("fail")
		}
		fmt.Printf("  %-11s %-4s %s\n", check.Name+":", label, check.Message)
	}

	if report.OK {
		return 0
	}
	return 1
}

func resolveLaunchTarget(repo *gitcore.Repository, parsed appFlags) (launchTarget, error) {
	target := launchTarget{
		Path: parsed.targetPath,
	}
	if parsed.branch != "" && parsed.targetRev != "" {
		return target, fmt.Errorf("use either --branch or --commit, not both")
	}
	if parsed.branch != "" {
		hash, ok := repo.Branches()[parsed.branch]
		if !ok {
			return target, fmt.Errorf("unknown branch: %s", parsed.branch)
		}
		target.CommitHash = hash
	}
	if parsed.targetRev != "" {
		hash, err := resolveHash(repo, parsed.targetRev)
		if err != nil {
			return target, err
		}
		target.CommitHash = hash
	}
	if target.Path != "" && target.CommitHash == "" {
		head := repo.Head()
		if head == "" {
			return target, fmt.Errorf("HEAD is not set")
		}
		target.CommitHash = head
	}
	return target, nil
}

func buildURLs(addr string, target launchTarget) (string, string) {
	base := (&neturl.URL{
		Scheme: "http",
		Host:   addr,
	}).String()
	launch := &neturl.URL{
		Scheme: "http",
		Host:   addr,
	}
	if target.Path != "" {
		q := launch.Query()
		q.Set("path", target.Path)
		launch.RawQuery = q.Encode()
	}
	if target.CommitHash != "" {
		launch.Fragment = string(target.CommitHash)
	}
	return base, launch.String()
}

func printStartupBanner(cw *cli.Writer, command, baseURL, openURL, repoPath string, repoLoadDur time.Duration, launchBrowser bool) {
	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Printf("  %s    %s\n", cw.Cyan("cmd:"), command)
	timing := fmt.Sprintf("loaded in %s", cw.Yellow(repoLoadDur.String()))
	fmt.Printf("  %s   %s  (%s)\n", cw.Cyan("repo:"), repoPath, timing)
	fmt.Printf("  %s %s\n", cw.Cyan("listen:"), baseURL)
	if openURL != baseURL {
		fmt.Printf("  %s   %s\n", cw.Cyan("open:"), openURL)
	}
	if launchBrowser {
		fmt.Printf("  %s %s\n", cw.Cyan("browser:"), "launching")
	}
	fmt.Printf("  %s %s\n", cw.Cyan("commit:"), commit)
	if cli.IsTerminal(os.Stdout.Fd()) {
		fmt.Printf("\n%s\n", cw.Bold("Press Ctrl+C to stop."))
	}
}

func printStartupJSON(command, baseURL, openURL, repoPath string, repoLoadDur time.Duration) {
	info := startupInfo{
		Version:    version,
		Commit:     commit,
		BuildDate:  buildDate,
		Listen:     baseURL,
		OpenURL:    openURL,
		RepoPath:   repoPath,
		RepoLoadMs: repoLoadDur.Milliseconds(),
		Command:    command,
	}
	data, _ := json.Marshal(info)
	fmt.Println(string(data))
}

func printHelp(cw *cli.Writer, command string) {
	printFlag := func(flagText, description string) {
		fmt.Printf("  %-24s %s\n", cw.Flag(flagText), description)
	}

	if command != "" && command != commandHelp {
		printCommandHelp(cw, command, printFlag)
		return
	}

	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Println("Real-time local Git repository visualization")
	fmt.Println()
	fmt.Println(cw.Bold("Usage:"))
	fmt.Println("  gitvista [flags]")
	fmt.Println("  gitvista open [flags]")
	fmt.Println("  gitvista serve [flags]")
	fmt.Println("  gitvista url [flags]")
	fmt.Println("  gitvista doctor [flags]")
	fmt.Println("  gitvista update")
	fmt.Println("  gitvista help [command]")
	fmt.Println()
	fmt.Println(cw.Bold("Commands:"))
	fmt.Println("  help     Show general or command-specific help")
	fmt.Println("  open     Start GitVista and launch the browser")
	fmt.Println("  serve    Start GitVista without launching the browser")
	fmt.Println("  url      Print the resolved launch URL")
	fmt.Println("  doctor   Validate repo, listener, and browser readiness")
	fmt.Println("  update   Download and install the latest release")
	fmt.Println()
	fmt.Println(cw.Bold("Global flags:"))
	printFlag("-repo <path>", "Path to git repository (default: current directory)")
	printFlag("-port, --port <port>", "Port to listen on (default: 8080)")
	printFlag("-host <host>", "Host to bind to (default: 127.0.0.1)")
	printFlag("-color <mode>", "Color output: auto, always, never")
	printFlag("-no-color", "Disable color output")
	printFlag("-version", "Show version and exit")
	printFlag("-check-update", "Check for a newer release and exit")
	printFlag("-help, -h", "Show help and exit")
	fmt.Println()

	switch command {
	case commandOpen:
		fmt.Println(cw.Bold("Open flags:"))
		printFlag("-commit <rev>", "Open focused on a commit or revision")
		printFlag("-branch <name>", "Open focused on a branch tip")
		printFlag("-path <path>", "Open the file explorer focused on a path")
		printFlag("-no-browser", "Start the server without opening a browser")
		printFlag("-print-url", "Print the resolved launch URL")
		printFlag("-output <format>", "Startup output format: json")
		fmt.Println()
	case commandServe:
		fmt.Println(cw.Bold("Serve flags:"))
		printFlag("-output <format>", "Startup output format: json")
		fmt.Println()
	case commandURL:
		fmt.Println(cw.Bold("URL flags:"))
		printFlag("-commit <rev>", "Build a URL focused on a commit or revision")
		printFlag("-branch <name>", "Build a URL focused on a branch tip")
		printFlag("-path <path>", "Build a URL focused on a path")
		printFlag("-json", "Print structured JSON output")
		fmt.Println()
	case commandDoctor:
		fmt.Println(cw.Bold("Doctor flags:"))
		printFlag("-json", "Print structured JSON output")
		fmt.Println()
	}

	fmt.Println(cw.Bold("Examples:"))
	fmt.Println("  gitvista")
	fmt.Println("  gitvista open --branch main")
	fmt.Println("  gitvista open --path internal/server")
	fmt.Println("  gitvista serve --port 3000")
	fmt.Println("  gitvista url --commit HEAD~1")
	fmt.Println("  gitvista doctor")
	fmt.Println("  gitvista update")
	fmt.Println()
	fmt.Println(cw.Bold("Environment Variables:"))
	fmt.Println("  GITVISTA_REPO         Repository path (default: current directory)")
	fmt.Println("  GITVISTA_PORT         Default port")
	fmt.Println("  GITVISTA_HOST         Default host")
	fmt.Println("  GITVISTA_LOG_LEVEL    Log level: debug, info, warn, error (default: info)")
	fmt.Println("  GITVISTA_LOG_FORMAT   Log format: text, json (default: text)")
}

func printCommandHelp(cw *cli.Writer, command string, printFlag func(string, string)) {
	fmt.Printf("%s %s\n", cw.Command("GitVista"), cw.Muted(version))
	fmt.Println()

	switch command {
	case commandOpen:
		fmt.Println(cw.Bold("Usage:"))
		fmt.Println("  gitvista open [flags] [revision]")
		fmt.Println()
		fmt.Println("Start GitVista and launch the browser.")
		fmt.Println()
		fmt.Println(cw.Bold("Flags:"))
		printFlag("-repo <path>", "Path to git repository (default: current directory)")
		printFlag("-port, --port <port>", "Port to listen on (default: 8080)")
		printFlag("-host <host>", "Host to bind to (default: 127.0.0.1)")
		printFlag("-color <mode>", "Color output: auto, always, never")
		printFlag("-no-color", "Disable color output")
		printFlag("-version", "Show version and exit")
		printFlag("-check-update", "Check for a newer release and exit")
		printFlag("-help, -h", "Show help and exit")
		printFlag("-commit <rev>", "Open focused on a commit or revision")
		printFlag("-branch <name>", "Open focused on a branch tip")
		printFlag("-path <path>", "Open the file explorer focused on a path")
		printFlag("-no-browser", "Start the server without opening a browser")
		printFlag("-print-url", "Print the resolved launch URL")
		printFlag("-output <format>", "Startup output format: json")
		fmt.Println()
		fmt.Println(cw.Bold("Examples:"))
		fmt.Println("  gitvista open")
		fmt.Println("  gitvista open HEAD~2")
		fmt.Println("  gitvista open --branch main")
		fmt.Println("  gitvista open --path internal/server")
	case commandServe:
		fmt.Println(cw.Bold("Usage:"))
		fmt.Println("  gitvista serve [flags]")
		fmt.Println()
		fmt.Println("Start GitVista without launching the browser.")
		fmt.Println()
		fmt.Println(cw.Bold("Flags:"))
		printFlag("-repo <path>", "Path to git repository (default: current directory)")
		printFlag("-port, --port <port>", "Port to listen on (default: 8080)")
		printFlag("-host <host>", "Host to bind to (default: 127.0.0.1)")
		printFlag("-color <mode>", "Color output: auto, always, never")
		printFlag("-no-color", "Disable color output")
		printFlag("-version", "Show version and exit")
		printFlag("-check-update", "Check for a newer release and exit")
		printFlag("-help, -h", "Show help and exit")
		printFlag("-output <format>", "Startup output format: json")
		fmt.Println()
		fmt.Println(cw.Bold("Examples:"))
		fmt.Println("  gitvista serve")
		fmt.Println("  gitvista serve --port 3000")
	case commandURL:
		fmt.Println(cw.Bold("Usage:"))
		fmt.Println("  gitvista url [flags]")
		fmt.Println()
		fmt.Println("Print the resolved launch URL.")
		fmt.Println()
		fmt.Println(cw.Bold("Flags:"))
		printFlag("-repo <path>", "Path to git repository (default: current directory)")
		printFlag("-port, --port <port>", "Port to listen on (default: 8080)")
		printFlag("-host <host>", "Host to bind to (default: 127.0.0.1)")
		printFlag("-color <mode>", "Color output: auto, always, never")
		printFlag("-no-color", "Disable color output")
		printFlag("-version", "Show version and exit")
		printFlag("-check-update", "Check for a newer release and exit")
		printFlag("-help, -h", "Show help and exit")
		printFlag("-commit <rev>", "Build a URL focused on a commit or revision")
		printFlag("-branch <name>", "Build a URL focused on a branch tip")
		printFlag("-path <path>", "Build a URL focused on a path")
		printFlag("-json", "Print structured JSON output")
		fmt.Println()
		fmt.Println(cw.Bold("Examples:"))
		fmt.Println("  gitvista url")
		fmt.Println("  gitvista url --commit HEAD~1")
		fmt.Println("  gitvista url --branch main --json")
	case commandDoctor:
		fmt.Println(cw.Bold("Usage:"))
		fmt.Println("  gitvista doctor [flags]")
		fmt.Println()
		fmt.Println("Validate repo, listener, and browser readiness.")
		fmt.Println()
		fmt.Println(cw.Bold("Flags:"))
		printFlag("-repo <path>", "Path to git repository (default: current directory)")
		printFlag("-port, --port <port>", "Port to listen on (default: 8080)")
		printFlag("-host <host>", "Host to bind to (default: 127.0.0.1)")
		printFlag("-color <mode>", "Color output: auto, always, never")
		printFlag("-no-color", "Disable color output")
		printFlag("-version", "Show version and exit")
		printFlag("-check-update", "Check for a newer release and exit")
		printFlag("-help, -h", "Show help and exit")
		printFlag("-json", "Print structured JSON output")
		fmt.Println()
		fmt.Println(cw.Bold("Examples:"))
		fmt.Println("  gitvista doctor")
		fmt.Println("  gitvista doctor --json")
	case commandUpdate:
		fmt.Println(cw.Bold("Usage:"))
		fmt.Println("  gitvista update")
		fmt.Println()
		fmt.Println("Download and install the latest release.")
		fmt.Println()
		fmt.Println(cw.Bold("Behavior:"))
		fmt.Println("  Refuses Homebrew-managed installs and tells you to use brew upgrade gitvista.")
		fmt.Println("  Replaces direct binary installs in place after checksum verification.")
		fmt.Println()
		fmt.Println(cw.Bold("Examples:"))
		fmt.Println("  gitvista update")
	}
}

func resolveHash(repo *gitcore.Repository, rev string) (gitcore.Hash, error) {
	if rev == "HEAD" {
		h := repo.Head()
		if h == "" {
			return "", fmt.Errorf("HEAD is not set")
		}
		return h, nil
	}

	if len(rev) == 40 {
		if _, err := gitcore.NewHash(rev); err == nil {
			return gitcore.Hash(rev), nil
		}
	}

	if hash, ok := repo.Branches()[rev]; ok {
		return hash, nil
	}

	if hashStr, ok := repo.Tags()[rev]; ok {
		return gitcore.Hash(hashStr), nil
	}

	if strings.HasPrefix(rev, "HEAD~") {
		n, err := strconv.Atoi(strings.TrimPrefix(rev, "HEAD~"))
		if err != nil {
			return "", fmt.Errorf("unknown revision: %s", rev)
		}
		current := repo.Head()
		if current == "" {
			return "", fmt.Errorf("HEAD is not set")
		}
		for i := 0; i < n; i++ {
			commit, err := repo.GetCommit(current)
			if err != nil {
				return "", err
			}
			if len(commit.Parents) == 0 {
				return "", fmt.Errorf("unknown revision: %s", rev)
			}
			current = commit.Parents[0]
		}
		return current, nil
	}

	if len(rev) >= 4 && len(rev) < 40 {
		commits := repo.Commits()
		var match gitcore.Hash
		count := 0
		for hash := range commits {
			if strings.HasPrefix(string(hash), rev) {
				match = hash
				count++
				if count > 1 {
					return "", fmt.Errorf("short hash %q is ambiguous", rev)
				}
			}
		}
		if count == 1 {
			return match, nil
		}
	}

	return "", fmt.Errorf("unknown revision: %s", rev)
}

func browserLauncher() ([]string, error) {
	switch runtime.GOOS {
	case "darwin":
		path, err := exec.LookPath("open")
		if err != nil {
			return nil, fmt.Errorf("browser launcher not found: open")
		}
		return []string{path}, nil
	case "linux":
		path, err := exec.LookPath("xdg-open")
		if err != nil {
			return nil, fmt.Errorf("browser launcher not found: xdg-open")
		}
		return []string{path}, nil
	case "windows":
		path, err := exec.LookPath("rundll32")
		if err != nil {
			return nil, fmt.Errorf("browser launcher not found: rundll32")
		}
		return []string{path, "url.dll,FileProtocolHandler"}, nil
	default:
		return nil, fmt.Errorf("browser launch is unsupported on %s", runtime.GOOS)
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		// #nosec G204,G702 -- launches a fixed local browser opener with an application-generated URL.
		return exec.Command("open", url).Start()
	case "linux":
		// #nosec G204,G702 -- launches a fixed local browser opener with an application-generated URL.
		return exec.Command("xdg-open", url).Start()
	case "windows":
		// #nosec G204,G702 -- launches a fixed local browser opener with an application-generated URL.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return fmt.Errorf("browser launch is unsupported on %s", runtime.GOOS)
	}
}
