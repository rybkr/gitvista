package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/cli"
)

func runRevList(repoCtx *repositoryContext, args []string, _ *cli.Writer) int {
	opts, exitCode, err := parseRevListArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode
	}

	commits, exitCode, err := revList(repoCtx.repo, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitCode
	}

	if opts.count {
		fmt.Println(len(commits))
		return 0
	}

	for _, commit := range commits {
		fmt.Fprintln(os.Stdout, commit.ID)
	}

	return 0
}

type revListOrder int

const (
	revListOrderChronological revListOrder = iota
	revListOrderTopo
	revListOrderDate
)

type revListOptions struct {
	all       bool
	revision  string
	count     bool
	noMerges  bool
	orderMode revListOrder
}

func parseRevListArgs(args []string) (revListOptions, int, error) {
	if len(args) == 0 {
		return revListOptions{}, 1, fmt.Errorf("usage: gitvista-cli rev-list [--all | <commit>] [--count] [--no-merges] [--topo-order] [--date-order]")
	}

	opts := revListOptions{orderMode: revListOrderChronological}
	for _, arg := range args {
		switch arg {
		case "--all":
			opts.all = true
		case "--count":
			opts.count = true
		case "--no-merges":
			opts.noMerges = true
		case "--topo-order":
			opts.orderMode = revListOrderTopo
		case "--date-order":
			opts.orderMode = revListOrderDate
		default:
			if strings.HasPrefix(arg, "--") {
				return revListOptions{}, 1, fmt.Errorf("gitvista-cli rev-list: unsupported argument %q", arg)
			}
			if opts.revision != "" {
				return revListOptions{}, 1, fmt.Errorf("gitvista-cli rev-list: accepts at most one revision argument")
			}
			opts.revision = arg
		}
	}

	if !opts.all && opts.revision == "" {
		return revListOptions{}, 1, fmt.Errorf("gitvista-cli rev-list: missing revision (expected <commit> or --all)")
	}

	return opts, 0, nil
}

func revList(repo *gitcore.Repository, opts revListOptions) ([]*gitcore.Commit, int, error) {
	commits, err := repo.RevList(gitcore.RevListOptions{
		All:      opts.all,
		Revision: opts.revision,
		NoMerges: opts.noMerges,
		Order:    mapRevListOrder(opts.orderMode),
	})
	if err != nil {
		return nil, 128, err
	}
	return commits, 0, nil
}

func mapRevListOrder(order revListOrder) gitcore.RevListOrder {
	switch order {
	case revListOrderTopo:
		return gitcore.RevListOrderTopo
	case revListOrderDate:
		return gitcore.RevListOrderDate
	default:
		return gitcore.RevListOrderChronological
	}
}
