package main

import (
	"container/heap"
	"fmt"
	"os"
	"sort"
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
	starts, exitCode, err := revListStartPoints(repo, opts)
	if err != nil {
		return nil, exitCode, err
	}
	if len(starts) == 0 {
		return nil, 0, nil
	}

	ordered := orderRevListCommits(repo.Commits(), starts, opts.orderMode)
	if !opts.noMerges {
		return ordered, 0, nil
	}

	filtered := ordered[:0]
	for _, commit := range ordered {
		if len(commit.Parents) <= 1 {
			filtered = append(filtered, commit)
		}
	}
	return filtered, 0, nil
}

func revListStartPoints(repo *gitcore.Repository, opts revListOptions) ([]gitcore.Hash, int, error) {
	seen := make(map[gitcore.Hash]struct{})
	var starts []gitcore.Hash

	add := func(hash gitcore.Hash) {
		if hash == "" {
			return
		}
		if _, ok := seen[hash]; ok {
			return
		}
		seen[hash] = struct{}{}
		starts = append(starts, hash)
	}

	if opts.all {
		branches := repo.GraphBranches()
		branchNames := make([]string, 0, len(branches))
		for name := range branches {
			branchNames = append(branchNames, name)
		}
		sort.Strings(branchNames)
		for _, name := range branchNames {
			add(branches[name])
		}

		tags := repo.Tags()
		tagNames := make([]string, 0, len(tags))
		for name := range tags {
			tagNames = append(tagNames, name)
		}
		sort.Strings(tagNames)
		for _, name := range tagNames {
			add(gitcore.Hash(tags[name]))
		}
	}

	if opts.revision != "" {
		hash, err := resolveRevision(repo, opts.revision)
		if err != nil {
			return nil, 128, err
		}
		add(hash)
	}

	return starts, 0, nil
}

func resolveRevision(repo *gitcore.Repository, revision string) (gitcore.Hash, error) {
	if revision == "HEAD" {
		head := repo.Head()
		if head == "" {
			return "", fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
		}
		return head, nil
	}

	if hash, ok := repo.Branches()[revision]; ok {
		return hash, nil
	}

	if hash, ok := repo.Tags()[revision]; ok {
		return gitcore.Hash(hash), nil
	}

	if hash, ok := repo.GraphBranches()[revision]; ok {
		return hash, nil
	}

	commits := repo.Commits()
	if commit, ok := commits[gitcore.Hash(revision)]; ok {
		return commit.ID, nil
	}

	var matches []gitcore.Hash
	for hash, commit := range commits {
		if strings.HasPrefix(string(hash), revision) {
			matches = append(matches, commit.ID)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	return "", fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
}

func collectReachableCommits(commits map[gitcore.Hash]*gitcore.Commit, starts []gitcore.Hash) map[gitcore.Hash]*gitcore.Commit {
	reachable := make(map[gitcore.Hash]*gitcore.Commit)
	stack := append([]gitcore.Hash(nil), starts...)

	for len(stack) > 0 {
		hash := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if _, ok := reachable[hash]; ok {
			continue
		}
		commit, ok := commits[hash]
		if !ok {
			continue
		}

		reachable[hash] = commit
		stack = append(stack, commit.Parents...)
	}

	return reachable
}

func orderRevListCommits(commits map[gitcore.Hash]*gitcore.Commit, starts []gitcore.Hash, mode revListOrder) []*gitcore.Commit {
	switch mode {
	case revListOrderTopo:
		return topoOrderRevListCommits(collectReachableCommits(commits, starts), false)
	case revListOrderDate:
		return topoOrderRevListCommits(collectReachableCommits(commits, starts), true)
	default:
		return chronologicalRevListCommits(commits, starts)
	}
}

func chronologicalRevListCommits(commits map[gitcore.Hash]*gitcore.Commit, starts []gitcore.Hash) []*gitcore.Commit {
	h := &revListCommitHeap{}
	heap.Init(h)
	nextSeq := 0

	seen := make(map[gitcore.Hash]struct{}, len(commits))
	for _, start := range starts {
		commit, ok := commits[start]
		if !ok {
			continue
		}
		if _, ok := seen[start]; ok {
			continue
		}
		seen[start] = struct{}{}
		heap.Push(h, revListCommitItem{commit: commit, seq: nextSeq})
		nextSeq++
	}

	ordered := make([]*gitcore.Commit, 0, len(commits))
	for h.Len() > 0 {
		commit := heap.Pop(h).(revListCommitItem).commit
		ordered = append(ordered, commit)
		for _, parent := range commit.Parents {
			parentCommit, ok := commits[parent]
			if !ok {
				continue
			}
			if _, ok := seen[parent]; ok {
				continue
			}
			seen[parent] = struct{}{}
			heap.Push(h, revListCommitItem{commit: parentCommit, seq: nextSeq})
			nextSeq++
		}
	}

	return ordered
}

func topoOrderRevListCommits(commits map[gitcore.Hash]*gitcore.Commit, useDate bool) []*gitcore.Commit {
	childCount := make(map[gitcore.Hash]int, len(commits))
	for hash := range commits {
		childCount[hash] = 0
	}
	for _, commit := range commits {
		for _, parent := range commit.Parents {
			if _, ok := commits[parent]; ok {
				childCount[parent]++
			}
		}
	}

	if useDate {
		h := &revListCommitHeap{}
		heap.Init(h)
		nextSeq := 0
		for hash, count := range childCount {
			if count == 0 {
				heap.Push(h, revListCommitItem{commit: commits[hash], seq: nextSeq})
				nextSeq++
			}
		}

		ordered := make([]*gitcore.Commit, 0, len(commits))
		for h.Len() > 0 {
			commit := heap.Pop(h).(revListCommitItem).commit
			ordered = append(ordered, commit)
			for _, parent := range commit.Parents {
				count, ok := childCount[parent]
				if !ok {
					continue
				}
				count--
				childCount[parent] = count
				if count == 0 {
					heap.Push(h, revListCommitItem{commit: commits[parent], seq: nextSeq})
					nextSeq++
				}
			}
		}
		return ordered
	}

	var stack []*gitcore.Commit
	for hash, count := range childCount {
		if count == 0 {
			stack = append(stack, commits[hash])
		}
	}
	sort.Slice(stack, func(i, j int) bool {
		left := stack[i]
		right := stack[j]
		if !left.Committer.When.Equal(right.Committer.When) {
			return left.Committer.When.Before(right.Committer.When)
		}
		return left.ID < right.ID
	})

	ordered := make([]*gitcore.Commit, 0, len(commits))
	for len(stack) > 0 {
		last := len(stack) - 1
		commit := stack[last]
		stack = stack[:last]
		ordered = append(ordered, commit)

		ready := make([]*gitcore.Commit, 0, len(commit.Parents))
		for _, parent := range commit.Parents {
			count, ok := childCount[parent]
			if !ok {
				continue
			}
			count--
			childCount[parent] = count
			if count == 0 {
				ready = append(ready, commits[parent])
			}
		}
		sort.Slice(ready, func(i, j int) bool {
			left := ready[i]
			right := ready[j]
			if !left.Committer.When.Equal(right.Committer.When) {
				return left.Committer.When.Before(right.Committer.When)
			}
			return left.ID < right.ID
		})
		stack = append(stack, ready...)
	}

	return ordered
}

type revListCommitItem struct {
	commit *gitcore.Commit
	seq    int
}

type revListCommitHeap []revListCommitItem

func (h revListCommitHeap) Len() int {
	return len(h)
}

func (h revListCommitHeap) Less(i, j int) bool {
	left := h[i]
	right := h[j]
	leftCommit := left.commit
	rightCommit := right.commit
	if !leftCommit.Committer.When.Equal(rightCommit.Committer.When) {
		return leftCommit.Committer.When.After(rightCommit.Committer.When)
	}
	return left.seq < right.seq
}

func (h revListCommitHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *revListCommitHeap) Push(x any) {
	*h = append(*h, x.(revListCommitItem))
}

func (h *revListCommitHeap) Pop() any {
	old := *h
	last := len(old) - 1
	item := old[last]
	old[last] = revListCommitItem{}
	*h = old[:last]
	return item
}
