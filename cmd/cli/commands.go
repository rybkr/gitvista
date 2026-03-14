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
		Name:    "version",
		Summary: "Show version information",
		Usage:   "gitvista-cli version",
		Run:     func([]string) int { printVersion(cw); return 0 },
	})
}
