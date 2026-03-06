package main

import (
	"fmt"
	"os"

	"github.com/rybkr/gitvista/internal/cli"
	"github.com/rybkr/gitvista/internal/gitcore"
)

func runStash(repo *gitcore.Repository, args []string, _ *cli.Writer) int {
	if len(args) == 0 || args[0] != "list" {
		fmt.Fprintln(os.Stderr, "usage: gitvista-cli stash list")
		return 1
	}

	stashes := repo.Stashes()
	for i, s := range stashes {
		fmt.Printf("stash@{%d}: %s\n", i, s.Message)
	}

	return 0
}
