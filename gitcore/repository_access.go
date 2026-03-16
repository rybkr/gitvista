package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LsTreeOptions configures a Repository.LsTree lookup.
type LsTreeOptions struct {
	// Revision is the commit revision to inspect.
	Revision string
}

// Name returns the base name of the repository's working directory.
func (r *Repository) Name() string {
	return filepath.Base(r.workDir)
}

// GitDir returns the path to the repository's .git directory.
func (r *Repository) GitDir() string {
	return r.gitDir
}

// WorkDir returns the path to the repository's working directory.
func (r *Repository) WorkDir() string {
	return r.workDir
}

// IsBare reports whether the repository is a bare repository.
func (r *Repository) IsBare() bool {
	return r.gitDir == r.workDir
}

// Remotes parses .git/config and returns remote names to URLs (credentials stripped).
func (r *Repository) Remotes() map[string]string {
	configPath := filepath.Join(r.gitDir, "config")
	// #nosec G304 -- .git config path is controlled by repository location
	content, err := os.ReadFile(configPath)
	if err != nil {
		return make(map[string]string)
	}
	return parseRemotesFromConfig(string(content))
}

// Commits returns a copy of all commits keyed by hash.
func (r *Repository) Commits() map[Hash]*Commit {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[Hash]*Commit, len(r.commitMap))
	for hash, commit := range r.commitMap {
		result[hash] = commit
	}
	return result
}

// Branches returns a map of local branch short names to their tip commit hashes.
func (r *Repository) Branches() map[string]Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Hash)
	for ref, hash := range r.refs {
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			result[name] = hash
		}
	}
	return result
}

// GraphBranches returns graph-visible branch refs to their tip commit hashes.
func (r *Repository) GraphBranches() map[string]Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]Hash)
	for ref, hash := range r.refs {
		if strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/remotes/") {
			result[ref] = hash
		}
	}
	return result
}

// Tags returns tag names to target commit hashes.
func (r *Repository) Tags() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	annotatedTargets := make(map[Hash]Hash, len(r.tags))
	for _, tag := range r.tags {
		annotatedTargets[tag.ID] = tag.Object
	}

	result := make(map[string]string, len(r.refs))
	for ref, hash := range r.refs {
		name, ok := strings.CutPrefix(ref, "refs/tags/")
		if !ok {
			continue
		}
		if commitHash, isAnnotated := annotatedTargets[hash]; isAnnotated {
			result[name] = string(commitHash)
		} else {
			result[name] = string(hash)
		}
	}
	return result
}

// Stashes returns all stash entries in the repository.
func (r *Repository) Stashes() []*StashEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*StashEntry, len(r.stashes))
	copy(result, r.stashes)
	return result
}

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

// LsTree resolves a commit revision and returns the entries in its root tree.
func (r *Repository) LsTree(opts LsTreeOptions) ([]TreeEntry, error) {
	hash, err := r.ResolveRevision(opts.Revision)
	if err != nil {
		return nil, err
	}

	objectType, err := r.objectType(hash)
	if err != nil {
		return nil, err
	}
	if objectType != ObjectTypeCommit {
		return nil, fmt.Errorf("object %s is not a commit", hash)
	}

	commit, err := r.getCommit(hash)
	if err != nil {
		return nil, err
	}

	tree, err := r.getTree(commit.Tree)
	if err != nil {
		return nil, err
	}

	return append([]TreeEntry(nil), tree.Entries...), nil
}

func (r *Repository) getCommit(hash Hash) (*Commit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if commit, ok := r.commitMap[hash]; ok {
		return commit, nil
	}
	return nil, fmt.Errorf("commit not found: %s", hash)
}

func (r *Repository) getTree(treeHash Hash) (*Tree, error) {
	object, err := r.readObject(treeHash)
	if err != nil {
		return nil, fmt.Errorf("failed to read tree object: %w", err)
	}

	tree, ok := object.(*Tree)
	if !ok {
		return nil, fmt.Errorf("object %s is not a tree", treeHash)
	}

	return tree, nil
}

func (r *Repository) objectType(hash Hash) (ObjectType, error) {
	_, objectType, err := r.readObjectData(hash, 0)
	if err != nil {
		return ObjectTypeInvalid, err
	}
	return objectType, nil
}
