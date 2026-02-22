package main

import (
	"fmt"
	"os"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: gitvista-cli <command> [<args>]")
		fmt.Fprintln(os.Stderr, "commands: log, cat-file, diff, status")
		os.Exit(1)
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

	var exitCode int
	switch os.Args[1] {
	case "log":
		exitCode = runLog(repo, os.Args[2:])
	case "cat-file":
		exitCode = runCatFile(repo, os.Args[2:])
	case "diff":
		exitCode = runDiff(repo, os.Args[2:])
	case "status":
		exitCode = runStatus(repo, os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "gitvista-cli: %q is not a command\n", os.Args[1]) //nolint:gosec // G705: CLI stderr, not web; %q quotes safely
		exitCode = 1
	}
	os.Exit(exitCode)
}
