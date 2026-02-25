package gitcore

import (
	"container/heap"
	"fmt"
)

// MergeBase finds the best common ancestor of two commits using a
// bidirectional BFS with date-ordered priority queues.
// Returns an error if no common ancestor exists.
func MergeBase(repo *Repository, ours, theirs Hash) (Hash, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	cm := repo.commitsMap()

	oursCommit, ok := cm[ours]
	if !ok {
		return "", fmt.Errorf("commit not found: %s", ours)
	}
	theirsCommit, ok := cm[theirs]
	if !ok {
		return "", fmt.Errorf("commit not found: %s", theirs)
	}

	// Track which sides have visited each commit.
	// Bit 1 = ours, bit 2 = theirs.
	const sideOurs = 1
	const sideTheirs = 2

	visited := make(map[Hash]int)

	h := &commitHeap{}
	heap.Init(h)

	visited[ours] = sideOurs
	visited[theirs] |= sideTheirs

	heap.Push(h, oursCommit)
	if ours != theirs {
		heap.Push(h, theirsCommit)
	} else {
		return ours, nil
	}

	for h.Len() > 0 {
		c := heap.Pop(h).(*Commit) //nolint:errcheck

		side := visited[c.ID]
		if side == sideOurs|sideTheirs {
			return c.ID, nil
		}

		for _, parentHash := range c.Parents {
			prevSide := visited[parentHash]
			newSide := prevSide | side

			if newSide == sideOurs|sideTheirs {
				return parentHash, nil
			}

			if newSide != prevSide {
				visited[parentHash] = newSide
				if parent, found := cm[parentHash]; found {
					heap.Push(h, parent)
				}
			}
		}
	}

	return "", fmt.Errorf("no common ancestor between %s and %s", ours.Short(), theirs.Short())
}

// MergePreview computes a preview of merging theirs into ours without
// modifying the repository. It finds the merge base, diffs both sides
// against it, and classifies each changed file.
func MergePreview(repo *Repository, oursHash, theirsHash Hash) (*MergePreviewResult, error) {
	baseHash, err := MergeBase(repo, oursHash, theirsHash)
	if err != nil {
		return nil, err
	}

	// Look up commits to get tree hashes.
	oursCommit, err := repo.GetCommit(oursHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get ours commit: %w", err)
	}
	theirsCommit, err := repo.GetCommit(theirsHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get theirs commit: %w", err)
	}

	var baseTree Hash
	if baseHash != "" {
		baseCommit, err := repo.GetCommit(baseHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get base commit: %w", err)
		}
		baseTree = baseCommit.Tree
	}

	oursDiff, err := TreeDiff(repo, baseTree, oursCommit.Tree, "")
	if err != nil {
		return nil, fmt.Errorf("failed to diff ours against base: %w", err)
	}

	theirsDiff, err := TreeDiff(repo, baseTree, theirsCommit.Tree, "")
	if err != nil {
		return nil, fmt.Errorf("failed to diff theirs against base: %w", err)
	}

	// Index diffs by path.
	oursMap := make(map[string]DiffEntry, len(oursDiff))
	for _, e := range oursDiff {
		oursMap[e.Path] = e
	}
	theirsMap := make(map[string]DiffEntry, len(theirsDiff))
	for _, e := range theirsDiff {
		theirsMap[e.Path] = e
	}

	// Union of all changed paths.
	allPaths := make(map[string]struct{})
	for p := range oursMap {
		allPaths[p] = struct{}{}
	}
	for p := range theirsMap {
		allPaths[p] = struct{}{}
	}

	entries := make([]MergePreviewEntry, 0, len(allPaths))
	conflicts := 0

	for path := range allPaths {
		oursEntry, inOurs := oursMap[path]
		theirsEntry, inTheirs := theirsMap[path]

		entry := MergePreviewEntry{
			Path:     path,
			IsBinary: (inOurs && oursEntry.IsBinary) || (inTheirs && theirsEntry.IsBinary),
		}

		if inOurs {
			entry.OursStatus = oursEntry.Status.String()
			entry.OursHash = oursEntry.NewHash
			entry.BaseHash = oursEntry.OldHash
		}
		if inTheirs {
			entry.TheirsStatus = theirsEntry.Status.String()
			entry.TheirsHash = theirsEntry.NewHash
			if entry.BaseHash == "" {
				entry.BaseHash = theirsEntry.OldHash
			}
		}

		switch {
		case inOurs && !inTheirs:
			// Only ours changed — clean merge.
			entry.ConflictType = ConflictNone

		case !inOurs && inTheirs:
			// Only theirs changed — clean merge.
			entry.ConflictType = ConflictNone

		case inOurs && inTheirs:
			entry.ConflictType = classifyConflict(oursEntry, theirsEntry)
		}

		if entry.ConflictType != ConflictNone {
			conflicts++
		}

		entries = append(entries, entry)
	}

	return &MergePreviewResult{
		MergeBaseHash: baseHash,
		OursHash:      oursHash,
		TheirsHash:    theirsHash,
		Entries:       entries,
		Stats: MergePreviewStats{
			TotalFiles: len(entries),
			Conflicts:  conflicts,
			CleanMerge: len(entries) - conflicts,
		},
	}, nil
}

// classifyConflict determines the conflict type when both sides changed the same file.
func classifyConflict(ours, theirs DiffEntry) ConflictType {
	// Both sides made the same change (same resulting hash) — trivial merge.
	if ours.NewHash != "" && ours.NewHash == theirs.NewHash {
		return ConflictNone
	}

	// Both added the same path.
	if ours.Status == DiffStatusAdded && theirs.Status == DiffStatusAdded {
		return ConflictBothAdded
	}

	// One deleted, other modified.
	if (ours.Status == DiffStatusDeleted && theirs.Status != DiffStatusDeleted) ||
		(ours.Status != DiffStatusDeleted && theirs.Status == DiffStatusDeleted) {
		return ConflictDeleteModify
	}

	// Both deleted — no conflict.
	if ours.Status == DiffStatusDeleted && theirs.Status == DiffStatusDeleted {
		return ConflictNone
	}

	// Both modified to different hashes — content conflict.
	return ConflictConflicting
}
