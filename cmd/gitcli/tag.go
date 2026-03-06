package main

import (
	"fmt"
	"sort"

	"github.com/rybkr/gitvista/internal/cli"
	"github.com/rybkr/gitvista/internal/gitcore"
)

func runTag(repo *gitcore.Repository, _ []string, _ *cli.Writer) int {
	names := repo.TagNames()
	sort.Strings(names)

	for _, name := range names {
		fmt.Println(name)
	}

	return 0
}
