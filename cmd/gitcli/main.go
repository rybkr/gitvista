package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/termcolor"
)

// Build-time variables set via -ldflags.
var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	gf, args := parseGlobalFlags(os.Args[1:])

	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: gitvista-cli [--color=auto|always|never] [--no-color] <command> [<args>]")
		fmt.Fprintln(os.Stderr, "commands: log, cat-file, diff, status, version")
		os.Exit(1)
	}

	// Handle version before loading repo — no repo needed.
	if args[0] == "version" || args[0] == "--version" {
		printVersion()
		os.Exit(0)
	}

	repoPath := os.Getenv("GIT_DIR")
	if repoPath == "" {
		repoPath = "."
	}

	repo, err := gitcore.NewRepository(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(128)
	}

	_ = termcolor.NewWriter(os.Stdout, gf.colorMode) // Phase 2 wires this into output

	var exitCode int
	switch args[0] {
	case "log":
		exitCode = runLog(repo, args[1:])
	case "cat-file":
		exitCode = runCatFile(repo, args[1:])
	case "diff":
		exitCode = runDiff(repo, args[1:])
	case "status":
		exitCode = runStatus(repo, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "gitvista-cli: %q is not a command\n", args[0]) //nolint:gosec // G705: CLI stderr, not web; %q quotes safely
		exitCode = 1
	}
	os.Exit(exitCode)
}

func printVersion() {
	fmt.Printf("GitVista CLI %s\n", version)
	fmt.Printf("  commit:     %s\n", commit)
	fmt.Printf("  built:      %s\n", buildDate)
	fmt.Printf("  go version: %s\n", runtime.Version())
	fmt.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
