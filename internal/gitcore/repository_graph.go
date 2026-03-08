package gitcore

import "container/heap"

// BuildGraphSummary constructs a lightweight GraphSummary containing only the
// topology (parent hashes) and temporal data (committer timestamps) for every
// commit, plus branches, tags, HEAD, and stashes. This is much smaller than the
// full commit set and enables the client to compute graph layout without
// materializing heavyweight commit data.
func (r *Repository) BuildGraphSummary() *GraphSummary {
	// Build skeletons and read head under the lock, then release before
	// calling Branches/Tags/Stashes which acquire their own RLock.
	r.mu.RLock()

	skeletons := make([]CommitSkeleton, 0, len(r.commits))
	var oldest, newest int64
	for _, c := range r.commits {
		ts := c.Committer.When.Unix()
		skeletons = append(skeletons, CommitSkeleton{
			Hash:      c.ID,
			Parents:   c.Parents,
			Timestamp: ts,
		})
		if oldest == 0 || ts < oldest {
			oldest = ts
		}
		if ts > newest {
			newest = ts
		}
	}
	totalCommits := len(r.commits)
	headHash := string(r.head)
	r.mu.RUnlock()

	return &GraphSummary{
		TotalCommits:    totalCommits,
		Skeleton:        skeletons,
		Branches:        r.GraphBranches(),
		Tags:            r.Tags(),
		HeadHash:        headHash,
		Stashes:         r.Stashes(),
		OldestTimestamp: oldest,
		NewestTimestamp: newest,
	}
}

// commitHeap is a max-heap of commits sorted by committer date (newest first).
type commitHeap []*Commit

func (h commitHeap) Len() int {
	return len(h)
}

func (h commitHeap) Less(i, j int) bool {
	return h[i].Committer.When.After(h[j].Committer.When)
}

func (h commitHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *commitHeap) Push(x any) {
	*h = append(*h, x.(*Commit)) //nolint:errcheck // heap only stores *Commit; assertion always succeeds
}

func (h *commitHeap) Pop() any {
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

	cm := r.commitsMap()
	headCommit, ok := cm[r.head]
	if !ok {
		return nil
	}

	visited := make(map[Hash]bool)
	h := &commitHeap{}
	heap.Init(h)
	heap.Push(h, headCommit)
	visited[headCommit.ID] = true

	var result []*Commit
	for h.Len() > 0 {
		if maxCount > 0 && len(result) >= maxCount {
			break
		}
		c := heap.Pop(h).(*Commit) //nolint:errcheck // heap only stores *Commit; assertion always succeeds
		result = append(result, c)

		for _, parentHash := range c.Parents {
			if visited[parentHash] {
				continue
			}
			visited[parentHash] = true
			if parent, found := cm[parentHash]; found {
				heap.Push(h, parent)
			}
		}
	}
	return result
}
