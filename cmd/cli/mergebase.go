package main

import (
	"fmt"
	"os"

	"github.com/rybkr/gitvista/gitcore"
)

type mergeBaseOptions struct {
	ours   string
	theirs string
}

func runMergeBase(repoCtx *repositoryContext, args []string) int {
	opts, exitCode, err := parseMergeBaseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode
	}

	ours, err := repoCtx.repo.ResolveRevision(opts.ours)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 128
	}

	theirs, err := repoCtx.repo.ResolveRevision(opts.theirs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 128
	}

	base, err := gitcore.MergeBase(repoCtx.repo, ours, theirs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 128
	}

	fmt.Fprintln(os.Stdout, base)
	return 0
}

func parseMergeBaseArgs(args []string) (mergeBaseOptions, int, error) {
	if len(args) != 2 {
		return mergeBaseOptions{}, 1, fmt.Errorf("usage: gitvista-cli merge-base <commit> <commit>")
	}

	return mergeBaseOptions{
		ours:   args[0],
		theirs: args[1],
	}, 0, nil
}
