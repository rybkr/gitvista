package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/rybkr/gitvista/internal/cli"
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

	// --version is handled before app.Run because "--" prefixed args
	// would be treated as unknown commands by the dispatcher.
	for _, a := range args {
		if a == "--version" {
			printVersion()
			os.Exit(0)
		}
	}

	cw := termcolor.NewWriter(os.Stdout, gf.colorMode)

	app := cli.NewApp("gitvista-cli", version)
	app.Stderr = os.Stderr

	// repo is declared here and assigned after dispatch determines that
	// the matched command needs it (NeedsRepo). Closures capture the
	// pointer variable, which is populated before they execute.
	var repo *gitcore.Repository

	app.Register(&cli.Command{
		Name:      "branch",
		Summary:   "List branches",
		Usage:     "gitvista-cli branch",
		NeedsRepo: true,
		Run:       func(args []string) int { return runBranch(repo, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "log",
		Summary:   "Show commit log",
		Usage:     "gitvista-cli log [--oneline] [-n <count>]",
		Examples:  []string{"gitvista-cli log", "gitvista-cli log --oneline -n5"},
		NeedsRepo: true,
		Run:       func(args []string) int { return runLog(repo, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "cat-file",
		Summary:   "Show object content, type, or size",
		Usage:     "gitvista-cli cat-file (-t|-s|-p) <object>",
		Examples:  []string{"gitvista-cli cat-file -p HEAD", "gitvista-cli cat-file -t abc1234"},
		NeedsRepo: true,
		Run:       func(args []string) int { return runCatFile(repo, args) },
	})

	app.Register(&cli.Command{
		Name:      "diff",
		Summary:   "Show diff between two commits",
		Usage:     "gitvista-cli diff [--stat] <commit1> <commit2>",
		Examples:  []string{"gitvista-cli diff HEAD~1 HEAD", "gitvista-cli diff --stat main dev"},
		NeedsRepo: true,
		Run:       func(args []string) int { return runDiff(repo, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "show",
		Summary:   "Show commit details and diff",
		Usage:     "gitvista-cli show [--stat] [<commit>]",
		Examples:  []string{"gitvista-cli show", "gitvista-cli show --stat HEAD"},
		NeedsRepo: true,
		Run:       func(args []string) int { return runShow(repo, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "stash",
		Summary:   "List stash entries",
		Usage:     "gitvista-cli stash list",
		NeedsRepo: true,
		Run:       func(args []string) int { return runStash(repo, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "status",
		Summary:   "Show working tree status",
		Usage:     "gitvista-cli status [-s|--porcelain]",
		Examples:  []string{"gitvista-cli status", "gitvista-cli status --porcelain"},
		NeedsRepo: true,
		Run:       func(args []string) int { return runStatus(repo, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "tag",
		Summary:   "List tags",
		Usage:     "gitvista-cli tag",
		NeedsRepo: true,
		Run:       func(args []string) int { return runTag(repo, args, cw) },
	})

	app.Register(&cli.Command{
		Name:    "update",
		Summary: "Update to the latest release",
		Usage:   "gitvista-cli update [--check]",
		Examples: []string{
			"gitvista-cli update",
			"gitvista-cli update --check",
		},
		Run: func(args []string) int { return runUpdate(args) },
	})

	app.Register(&cli.Command{
		Name:    "version",
		Summary: "Show version information",
		Usage:   "gitvista-cli version",
		Run:     func([]string) int { printVersion(); return 0 },
	})

	// Determine which command will run so we can load the repo only when needed.
	if len(args) > 0 {
		cmd := app.Lookup(args[0])
		if cmd != nil && cmd.NeedsRepo {
			repoPath := os.Getenv("GIT_DIR")
			if repoPath == "" {
				repoPath = "."
			}
			var err error
			repo, err = gitcore.NewRepository(repoPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
				os.Exit(128)
			}
		}
	}

	os.Exit(app.Run(args, cw))
}

func printVersion() {
	fmt.Printf("GitVista CLI %s\n", version)
	fmt.Printf("  commit:     %s\n", commit)
	fmt.Printf("  built:      %s\n", buildDate)
	fmt.Printf("  go version: %s\n", runtime.Version())
	fmt.Printf("  platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
