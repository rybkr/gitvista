package gitcore

import (
	"container/heap"
	"fmt"
)

type mergeBaseCommitHeap []*Commit

func (h mergeBaseCommitHeap) Len() int {
	return len(h)
}

func (h mergeBaseCommitHeap) Less(i, j int) bool {
	return h[i].Committer.When.After(h[j].Committer.When)
}

func (h mergeBaseCommitHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *mergeBaseCommitHeap) Push(x any) {
	*h = append(*h, x.(*Commit)) //nolint:errcheck
}

func (h *mergeBaseCommitHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}

// MergeBase finds the best common ancestor of two commits using a
// bidirectional BFS with date-ordered priority queues.
func MergeBase(repo *Repository, ours, theirs Hash) (Hash, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	commits := repo.commitMap

	oursCommit, ok := commits[ours]
	if !ok {
		return "", fmt.Errorf("commit not found: %s", ours)
	}
	theirsCommit, ok := commits[theirs]
	if !ok {
		return "", fmt.Errorf("commit not found: %s", theirs)
	}
	if ours == theirs {
		return ours, nil
	}

	const sideOurs = 1
	const sideTheirs = 2

	visited := map[Hash]int{
		ours:   sideOurs,
		theirs: sideTheirs,
	}

	h := &mergeBaseCommitHeap{}
	heap.Init(h)
	heap.Push(h, oursCommit)
	heap.Push(h, theirsCommit)

	for h.Len() > 0 {
		commit := heap.Pop(h).(*Commit) //nolint:errcheck
		side := visited[commit.ID]
		if side == sideOurs|sideTheirs {
			return commit.ID, nil
		}

		for _, parentHash := range commit.Parents {
			prevSide := visited[parentHash]
			newSide := prevSide | side
			if newSide == sideOurs|sideTheirs {
				return parentHash, nil
			}
			if newSide == prevSide {
				continue
			}
			visited[parentHash] = newSide
			if parent, found := commits[parentHash]; found {
				heap.Push(h, parent)
			}
		}
	}

	return "", fmt.Errorf("no common ancestor between %s and %s", ours.Short(), theirs.Short())
}
