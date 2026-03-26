package gitcore

import (
	"container/heap"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RevListOrder controls how `RevList` orders reachable commits.
type RevListOrder int

const (
	// RevListOrderChronological orders commits by committer date.
	RevListOrderChronological RevListOrder = iota
	// RevListOrderTopo preserves parent-before-ancestor topology.
	RevListOrderTopo
	// RevListOrderDate preserves topology while preferring newer commits when possible.
	RevListOrderDate
)

// RevListOptions configures revision traversal and filtering.
type RevListOptions struct {
	All      bool
	Revision string
	NoMerges bool
	Order    RevListOrder
}

// RevList returns commits reachable from the requested start points.
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

// ResolveRevision resolves a branch, tag, HEAD, or commit prefix to a commit hash.
func (r *Repository) ResolveRevision(revision string) (Hash, error) {
	if revision == "HEAD" {
		head := r.Head()
		if head == "" {
			return "", fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
		}
		return head, nil
	}

	if strings.HasPrefix(revision, "HEAD~") {
		r.mu.RLock()
		defer r.mu.RUnlock()

		if r.head == "" {
			return "", fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
		}

		distance, err := strconv.Atoi(strings.TrimPrefix(revision, "HEAD~"))
		if err != nil || distance < 0 {
			return "", fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
		}

		current := r.head
		for range distance {
			commit, ok := r.commitMap[current]
			if !ok || len(commit.Parents) == 0 {
				return "", fmt.Errorf("fatal: ambiguous argument %q: unknown revision or path not in the working tree", revision)
			}
			current = commit.Parents[0]
		}
		return current, nil
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

func orderRevListCommits(commits map[Hash]*Commit, starts []Hash, mode RevListOrder) []*Commit {
	reachable := chronologicalRevListCommits(commits, starts)

	switch mode {
	case RevListOrderTopo:
		return topoOrderRevListCommits(reachable, false)
	case RevListOrderDate:
		return topoOrderRevListCommits(reachable, true)
	default:
		return reachable
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
		commitItem := h.popItem()
		commit := commitItem.commit
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

func topoOrderRevListCommits(commits []*Commit, useDate bool) []*Commit {
	if len(commits) == 0 {
		return nil
	}

	commitByID := make(map[Hash]*Commit, len(commits))
	indegree := make(map[Hash]int, len(commits))
	for _, commit := range commits {
		commitByID[commit.ID] = commit
		indegree[commit.ID] = 1
	}
	for _, commit := range commits {
		for _, parent := range commit.Parents {
			if _, ok := commitByID[parent]; ok {
				indegree[parent]++
			}
		}
	}

	if useDate {
		h := &revListCommitHeap{}
		heap.Init(h)
		nextSeq := 0
		for _, commit := range commits {
			if indegree[commit.ID] == 1 {
				heap.Push(h, revListCommitItem{commit: commit, seq: nextSeq})
				nextSeq++
			}
		}

		ordered := make([]*Commit, 0, len(commits))
		for h.Len() > 0 {
			commit := h.popItem().commit
			ordered = append(ordered, commit)
			indegree[commit.ID] = 0
			for _, parent := range commit.Parents {
				count, ok := indegree[parent]
				if !ok {
					continue
				}
				count--
				indegree[parent] = count
				if count == 1 {
					heap.Push(h, revListCommitItem{commit: commitByID[parent], seq: nextSeq})
					nextSeq++
				}
			}
		}
		return ordered
	}

	var stack []*Commit
	for _, commit := range commits {
		if indegree[commit.ID] == 1 {
			stack = append(stack, commit)
		}
	}
	reverseRevListCommits(stack)

	ordered := make([]*Commit, 0, len(commits))
	for len(stack) > 0 {
		last := len(stack) - 1
		commit := stack[last]
		stack = stack[:last]
		ordered = append(ordered, commit)
		indegree[commit.ID] = 0

		for _, parent := range commit.Parents {
			count, ok := indegree[parent]
			if !ok {
				continue
			}
			count--
			indegree[parent] = count
			if count == 1 {
				stack = append(stack, commitByID[parent])
			}
		}
	}

	return ordered
}

func reverseRevListCommits(commits []*Commit) {
	for left, right := 0, len(commits)-1; left < right; left, right = left+1, right-1 {
		commits[left], commits[right] = commits[right], commits[left]
	}
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
	item, _ := x.(revListCommitItem)
	*h = append(*h, item)
}

func (h *revListCommitHeap) Pop() any {
	old := *h
	last := len(old) - 1
	item := old[last]
	old[last] = revListCommitItem{}
	*h = old[:last]
	return item
}

func (h *revListCommitHeap) popItem() revListCommitItem {
	item, _ := heap.Pop(h).(revListCommitItem)
	return item
}
