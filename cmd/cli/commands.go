package main

import (
	"time"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/cli"
)

type repositoryContext struct {
	path         string
	repo         *gitcore.Repository
	loadDuration time.Duration
}

func registerCommands(app *cli.App, repoCtx *repositoryContext, cw *cli.Writer) {
	app.Register(&cli.Command{
		Name:      "repo",
		Summary:   "Show repository summary",
		Usage:     "gitvista-cli repo",
		NeedsRepo: true,
		Run:       func(args []string) int { return runRepo(repoCtx, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "cat-file",
		Summary:   "Inspect git objects like git cat-file",
		Usage:     "gitvista-cli cat-file (-t | -s | -p) <object>",
		NeedsRepo: true,
		Flags: []string{
			"-t            Print the object type",
			"-s            Print the object size in bytes",
			"-p            Pretty-print the object",
			"<object>      A hash, short hash, branch, tag, remote ref, or HEAD",
		},
		Examples: []string{
			"Print the type of HEAD\ngitvista-cli cat-file -t HEAD",
			"Print the size of an object\ngitvista-cli cat-file -s a1b2c3d",
			"Pretty-print the information in HEAD\ngitvista-cli cat-file -p HEAD",
		},
		Run: func(args []string) int { return runCatFile(repoCtx, args) },
	})

	app.Register(&cli.Command{
		Name:      "rev-list",
		Summary:   "List commits like git rev-list",
		Usage:     "gitvista-cli rev-list [--all | <commit>] [--count] [--no-merges] [--topo-order] [--date-order]",
		NeedsRepo: true,
		Flags: []string{
			"--all         Walk from all branch and tag refs",
			"<commit>      Walk from a commit hash, short hash, branch, tag, or HEAD",
			"--count       Print only the number of selected commits",
			"--no-merges   Exclude merge commits from the output",
			"--topo-order  Keep parents after children in topological order",
			"--date-order  Keep topological constraints while preferring newer commits first",
		},
		Examples: []string{
			"List commits reachable from HEAD\ngitvista-cli rev-list HEAD",
			"Print how many commits are reachable from branch main\ngitvista-cli rev-list --count main",
			"List commits reachable from a specific commit\ngitvista-cli rev-list a1b2c3d",
			"List all refs in topological order\ngitvista-cli rev-list --all --topo-order",
		},
		Run: func(args []string) int { return runRevList(repoCtx, args, cw) },
	})

	app.Register(&cli.Command{
		Name:      "ls-tree",
		Summary:   "List a commit tree like git ls-tree",
		Usage:     "gitvista-cli ls-tree <commit>",
		NeedsRepo: true,
		Flags: []string{
			"<commit>      A commit hash, short hash, branch, tag, remote ref, or HEAD",
		},
		Examples: []string{
			"List the root tree for HEAD\ngitvista-cli ls-tree HEAD",
			"List the root tree for branch main\ngitvista-cli ls-tree main",
		},
		Run: func(args []string) int { return runLsTree(repoCtx, args) },
	})

	app.Register(&cli.Command{
		Name:    "version",
		Summary: "Show version information",
		Usage:   "gitvista-cli version",
		Run:     func([]string) int { printVersion(cw); return 0 },
	})
}
