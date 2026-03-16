package main

import (
	"fmt"
	"os"

	"github.com/rybkr/gitvista/gitcore"
)

type lsTreeOptions struct {
	revision string
}

func runLsTree(repoCtx *repositoryContext, args []string) int {
	opts, exitCode, err := parseLsTreeArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode
	}

	entries, err := repoCtx.repo.LsTree(gitcore.LsTreeOptions{Revision: opts.revision})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 128
	}

	for _, entry := range entries {
		fmt.Fprintf(os.Stdout, "%s %s %s\t%s\n", entry.Mode, entry.Type.String(), entry.ID, entry.Name)
	}

	return 0
}

func parseLsTreeArgs(args []string) (lsTreeOptions, int, error) {
	if len(args) != 1 {
		return lsTreeOptions{}, 1, fmt.Errorf("usage: gitvista-cli ls-tree <commit>")
	}

	if args[0] == "" {
		return lsTreeOptions{}, 1, fmt.Errorf("gitvista-cli ls-tree: missing commit")
	}

	return lsTreeOptions{revision: args[0]}, 0, nil
}
