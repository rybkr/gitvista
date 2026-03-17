package gitcore

import "container/heap"

type commitLogHeap []*Commit

func (h commitLogHeap) Len() int {
	return len(h)
}

func (h commitLogHeap) Less(i, j int) bool {
	return h[i].Committer.When.After(h[j].Committer.When)
}

func (h commitLogHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *commitLogHeap) Push(x any) {
	*h = append(*h, x.(*Commit)) //nolint:errcheck
}

func (h *commitLogHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return item
}

// CommitLog walks from HEAD through parents in reverse chronological order.
// If maxCount <= 0 all reachable commits are returned.
func (r *Repository) CommitLog(maxCount int) []*Commit {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.head == "" {
		return nil
	}

	headCommit, ok := r.commitMap[r.head]
	if !ok {
		return nil
	}

	visited := map[Hash]bool{headCommit.ID: true}
	h := &commitLogHeap{}
	heap.Init(h)
	heap.Push(h, headCommit)

	var result []*Commit
	for h.Len() > 0 {
		if maxCount > 0 && len(result) >= maxCount {
			break
		}

		commit := heap.Pop(h).(*Commit) //nolint:errcheck
		result = append(result, commit)

		for _, parentHash := range commit.Parents {
			if visited[parentHash] {
				continue
			}
			visited[parentHash] = true
			if parent, found := r.commitMap[parentHash]; found {
				heap.Push(h, parent)
			}
		}
	}

	return result
}
