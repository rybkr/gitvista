package gitcore

// CommitCount returns the number of commits loaded into the repository cache.
func (r *Repository) CommitCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.commits)
}

// BranchCount returns the number of local branches.
func (r *Repository) BranchCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for ref := range r.refs {
		if len(ref) > len("refs/heads/") && ref[:len("refs/heads/")] == "refs/heads/" {
			count++
		}
	}
	return count
}

// TagCount returns the number of tag refs.
func (r *Repository) TagCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for ref := range r.refs {
		if len(ref) > len("refs/tags/") && ref[:len("refs/tags/")] == "refs/tags/" {
			count++
		}
	}
	return count
}

// StashCount returns the number of recorded stash entries.
func (r *Repository) StashCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.stashes)
}

// Head returns the hash of the current HEAD commit.
func (r *Repository) Head() Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.head
}

// HeadRef returns the symbolic HEAD ref, or empty string when detached.
func (r *Repository) HeadRef() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headRef
}

// HeadDetached reports whether the repository is in detached HEAD state.
func (r *Repository) HeadDetached() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headDetached
}
