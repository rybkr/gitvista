package gitcore

import (
	"fmt"
	"slices"
)

// MergeBase finds a single best common ancestor of two commits.
func MergeBase(repo *Repository, ours, theirs Hash) (Hash, error) {
	bases, err := MergeBases(repo, ours, theirs)
	if err != nil {
		return "", err
	}
	return selectPreferredMergeBase(repo.commitMap, bases), nil
}

// MergeBases returns all best common ancestors of two commits.
func MergeBases(repo *Repository, ours, theirs Hash) ([]Hash, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	commits := repo.commitMap

	if _, ok := commits[ours]; !ok {
		return nil, fmt.Errorf("commit not found: %s", ours)
	}
	if _, ok := commits[theirs]; !ok {
		return nil, fmt.Errorf("commit not found: %s", theirs)
	}
	if ours == theirs {
		return []Hash{ours}, nil
	}

	common := collectCommonAncestors(commits, ours, theirs)
	if len(common) == 0 {
		return nil, fmt.Errorf("no common ancestor between %s and %s", ours.Short(), theirs.Short())
	}

	bases := bestCommonAncestors(commits, common)
	if len(bases) == 0 {
		return nil, fmt.Errorf("no common ancestor between %s and %s", ours.Short(), theirs.Short())
	}

	slices.SortFunc(bases, compareHashes)
	return bases, nil
}

func compareHashes(left, right Hash) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func collectCommonAncestors(commits map[Hash]*Commit, ours, theirs Hash) map[Hash]*Commit {
	oursAncestors := collectReachableCommits(commits, ours)
	theirsAncestors := collectReachableCommits(commits, theirs)

	common := make(map[Hash]*Commit)
	for hash := range oursAncestors {
		if _, ok := theirsAncestors[hash]; ok {
			common[hash] = commits[hash]
		}
	}
	return common
}

func collectReachableCommits(commits map[Hash]*Commit, start Hash) map[Hash]struct{} {
	reachable := make(map[Hash]struct{})
	stack := []Hash{start}

	for len(stack) > 0 {
		hash := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, seen := reachable[hash]; seen {
			continue
		}
		reachable[hash] = struct{}{}

		commit, ok := commits[hash]
		if !ok {
			continue
		}
		stack = append(stack, commit.Parents...)
	}

	return reachable
}

func bestCommonAncestors(commits map[Hash]*Commit, common map[Hash]*Commit) []Hash {
	redundant := make(map[Hash]struct{})
	for _, commit := range common {
		for _, parent := range commit.Parents {
			markCommonAncestors(commits, common, redundant, parent)
		}
	}

	bases := make([]Hash, 0, len(common))
	for hash := range common {
		if _, isRedundant := redundant[hash]; isRedundant {
			continue
		}
		bases = append(bases, hash)
	}
	return bases
}

func selectPreferredMergeBase(commits map[Hash]*Commit, bases []Hash) Hash {
	var best *Commit
	for _, hash := range bases {
		commit, ok := commits[hash]
		if !ok {
			continue
		}
		if best == nil || commit.Committer.When.After(best.Committer.When) {
			best = commit
			continue
		}
		if commit.Committer.When.Equal(best.Committer.When) && string(commit.ID) < string(best.ID) {
			best = commit
		}
	}
	if best == nil {
		return ""
	}
	return best.ID
}

func markCommonAncestors(commits map[Hash]*Commit, common map[Hash]*Commit, redundant map[Hash]struct{}, start Hash) {
	stack := []Hash{start}
	visited := make(map[Hash]struct{})

	for len(stack) > 0 {
		hash := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, seen := visited[hash]; seen {
			continue
		}
		visited[hash] = struct{}{}

		if _, ok := common[hash]; ok {
			redundant[hash] = struct{}{}
		}

		commit, ok := commits[hash]
		if !ok {
			continue
		}
		stack = append(stack, commit.Parents...)
	}
}
