package gitcore

// Diff computes a RepositoryDelta treating r as the new state and old as the previous state.
func (r *Repository) Diff(old *Repository) *RepositoryDelta {
	delta := NewRepositoryDelta()

	newCommits, oldCommits := r.Commits(), old.Commits()
	for hash, commit := range newCommits {
		if _, found := oldCommits[hash]; !found {
			delta.AddedCommits = append(delta.AddedCommits, commit)
		}
	}
	for hash, commit := range oldCommits {
		if _, found := newCommits[hash]; !found {
			delta.DeletedCommits = append(delta.DeletedCommits, commit)
		}
	}

	newBranches, oldBranches := r.Branches(), old.Branches()
	for branch, hash := range newBranches {
		if oldHash, found := oldBranches[branch]; !found {
			delta.AddedBranches[branch] = hash
		} else if hash != oldHash {
			delta.AmendedBranches[branch] = hash
		}
	}
	for branch, hash := range oldBranches {
		if _, found := newBranches[branch]; !found {
			delta.DeletedBranches[branch] = hash
		}
	}

	delta.HeadHash = string(r.Head())
	delta.Tags = r.Tags()
	delta.Stashes = r.Stashes()
	if delta.Stashes == nil {
		delta.Stashes = make([]*StashEntry, 0)
	}

	return delta
}
