package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/rybkr/gitvista/internal/cli"
)

func runRepo(repoCtx *repositoryContext, args []string, cw *cli.Writer) int {
	if len(args) != 0 {
		fmt.Printf("usage: gitvista-cli repo\n")
		return 1
	}

	repo := repoCtx.repo
	head := repo.Head().Short()
	if headRef := repo.HeadRef(); headRef != "" {
		head = strings.TrimPrefix(headRef, "refs/heads/")
	}

	repoType := "worktree"
	if repo.IsBare() {
		repoType = "bare"
	}

	fmt.Printf("%s %s\n", cw.Command("Repository"), cw.Muted(repo.Name()))
	fmt.Printf("  %s %s\n", cw.Cyan("worktree"), repo.WorkDir())
	fmt.Printf("  %s %s\n", cw.Cyan("git dir"), repo.GitDir())
	fmt.Printf("  %s %s\n", cw.Cyan("head"), head)
	fmt.Printf("  %s %s\n", cw.Cyan("type"), repoType)
	fmt.Println()
	fmt.Println(cw.Command("Load"))
	fmt.Printf("  %s %s\n", cw.Cyan("time"), repoCtx.loadDuration.Round(time.Millisecond))
	fmt.Println()
	fmt.Println(cw.Command("Stats"))
	fmt.Printf("  %s %d\n", cw.Cyan("commits"), repo.CommitCount())
	fmt.Printf("  %s %d\n", cw.Cyan("branches"), len(repo.Branches()))
	fmt.Printf("  %s %d\n", cw.Cyan("tags"), len(repo.Tags()))
	fmt.Printf("  %s %d\n", cw.Cyan("stashes"), len(repo.Stashes()))
	fmt.Printf("  %s %d\n", cw.Cyan("remotes"), len(repo.Remotes()))

	return 0
}
