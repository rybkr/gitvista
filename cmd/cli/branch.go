package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rybkr/gitvista/internal/cli"
	"github.com/rybkr/gitvista/internal/gitcore"
)

func runBranch(repo *gitcore.Repository, _ []string, cw *cli.Writer) int {
	branches := repo.Branches()

	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	sort.Strings(names)

	// Determine current branch from HEAD symbolic ref
	current := ""
	if ref := repo.HeadRef(); ref != "" {
		current = strings.TrimPrefix(ref, "refs/heads/")
	}

	for _, name := range names {
		if name == current {
			fmt.Printf("* %s\n", cw.Green(name))
		} else {
			fmt.Printf("  %s\n", name)
		}
	}

	return 0
}
