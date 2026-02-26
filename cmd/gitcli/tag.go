package main

import (
	"fmt"
	"sort"

	"github.com/rybkr/gitvista/internal/gitcore"
	"github.com/rybkr/gitvista/internal/termcolor"
)

func runTag(repo *gitcore.Repository, _ []string, _ *termcolor.Writer) int {
	names := repo.TagNames()
	sort.Strings(names)

	for _, name := range names {
		fmt.Println(name)
	}

	return 0
}
