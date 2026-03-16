package gitcore

import (
	"container/heap"
	"fmt"
	"sort"
	"strings"
)

type RevListOrder int

const (
	RevListOrderChronological RevListOrder = iota
	RevListOrderTopo
	RevListOrderDate
)

type RevListOptions struct {
	All      bool
	Revision string
	NoMerges bool
	Order    RevListOrder
}

func (r *Repository) RevList(opts RevListOptions) ([]*Commit, error) {
	starts, err := r.revListStartPoints(opts)
	if err != nil {
		return nil, err
	}
	if len(starts) == 0 {
		return nil, nil
	}

	ordered := orderRevListCommits(r.Commits(), starts, opts.Order)
	if !opts.NoMerges {
		return ordered, nil
	}

	filtered := ordered[:0]
	for _, commit := range ordered {
		if len(commit.Parents) <= 1 {
			filtered = append(filtered, commit)
		}
	}
	return filtered, nil
}

func (r *Repository) ResolveRevision(revision string) (Hash, error) {
	if revision == "HEAD" {
		head := r.Head()
		if head == "" {
			return "", fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
		}
		return head, nil
	}

	if hash, ok := r.Branches()[revision]; ok {
		return hash, nil
	}

	if hash, ok := r.Tags()[revision]; ok {
		return Hash(hash), nil
	}

	if hash, ok := r.GraphBranches()[revision]; ok {
		return hash, nil
	}

	commits := r.Commits()
	if commit, ok := commits[Hash(revision)]; ok {
		return commit.ID, nil
	}

	var matches []Hash
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

func (r *Repository) revListStartPoints(opts RevListOptions) ([]Hash, error) {
	seen := make(map[Hash]struct{})
	var starts []Hash

	add := func(hash Hash) {
		if hash == "" {
			return
		}
		if _, ok := seen[hash]; ok {
			return
		}
		seen[hash] = struct{}{}
		starts = append(starts, hash)
	}

	if opts.All {
		branches := r.GraphBranches()
		branchNames := make([]string, 0, len(branches))
		for name := range branches {
			branchNames = append(branchNames, name)
		}
		sort.Strings(branchNames)
		for _, name := range branchNames {
			add(branches[name])
		}

		tags := r.Tags()
		tagNames := make([]string, 0, len(tags))
		for name := range tags {
			tagNames = append(tagNames, name)
		}
		sort.Strings(tagNames)
		for _, name := range tagNames {
			add(Hash(tags[name]))
		}
	}

	if opts.Revision != "" {
		hash, err := r.ResolveRevision(opts.Revision)
		if err != nil {
			return nil, err
		}
		add(hash)
	}

	return starts, nil
}

func collectReachableCommits(commits map[Hash]*Commit, starts []Hash) map[Hash]*Commit {
	reachable := make(map[Hash]*Commit)
	stack := append([]Hash(nil), starts...)

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

func orderRevListCommits(commits map[Hash]*Commit, starts []Hash, mode RevListOrder) []*Commit {
	switch mode {
	case RevListOrderTopo:
		return topoOrderRevListCommits(collectReachableCommits(commits, starts), false)
	case RevListOrderDate:
		return topoOrderRevListCommits(collectReachableCommits(commits, starts), true)
	default:
		return chronologicalRevListCommits(commits, starts)
	}
}

func chronologicalRevListCommits(commits map[Hash]*Commit, starts []Hash) []*Commit {
	h := &revListCommitHeap{}
	heap.Init(h)
	nextSeq := 0

	seen := make(map[Hash]struct{}, len(commits))
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

	ordered := make([]*Commit, 0, len(commits))
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

func topoOrderRevListCommits(commits map[Hash]*Commit, useDate bool) []*Commit {
	childCount := make(map[Hash]int, len(commits))
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

		ordered := make([]*Commit, 0, len(commits))
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

	var stack []*Commit
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

	ordered := make([]*Commit, 0, len(commits))
	for len(stack) > 0 {
		last := len(stack) - 1
		commit := stack[last]
		stack = stack[:last]
		ordered = append(ordered, commit)

		ready := make([]*Commit, 0, len(commit.Parents))
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
	commit *Commit
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
