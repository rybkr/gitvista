package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// Commits returns a map of all commits in the repository keyed by their hash.
// The returned map is built once during construction and must not be modified.
func (r *Repository) Commits() map[Hash]*Commit {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.commitsMap()
}

// commitsMap returns the cached commit map. The map is always initialized by
// loadObjects during NewRepository construction; this method panics if that
// invariant is violated.
// Caller must hold at least r.mu.RLock().
func (r *Repository) commitsMap() map[Hash]*Commit {
	if r.commitMap == nil {
		panic("gitcore: commitMap is nil - Repository was not fully initialized via NewRepository")
	}
	return r.commitMap
}

// CommitCount returns the number of commits without building a map.
func (r *Repository) CommitCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.commits)
}

// Branches returns a map of branch names to their tip commit hashes.
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

// Head returns the hash of the current HEAD commit.
func (r *Repository) Head() Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.head
}

// HeadRef returns the symbolic ref (e.g., "refs/heads/main"), or empty string if detached.
func (r *Repository) HeadRef() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headRef
}

// HeadDetached reports whether the repository is in a detached HEAD state.
func (r *Repository) HeadDetached() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headDetached
}

// Description returns the .git/description contents, or empty string if
// the file is missing or contains Git's default placeholder text.
func (r *Repository) Description() string {
	descPath := filepath.Join(r.gitDir, "description")
	content, err := os.ReadFile(descPath)
	if err != nil {
		return ""
	}

	desc := strings.TrimSpace(string(content))
	if desc == "Unnamed repository; edit this file 'description' to name the repository." {
		return ""
	}

	return desc
}

// Remotes parses .git/config and returns remote names to URLs (credentials stripped).
func (r *Repository) Remotes() map[string]string {
	configPath := filepath.Join(r.gitDir, "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return make(map[string]string)
	}
	return parseRemotesFromConfig(string(content))
}

// TagNames returns a list of all tag names in the repository.
func (r *Repository) TagNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0)
	for ref := range r.refs {
		if name, ok := strings.CutPrefix(ref, "refs/tags/"); ok {
			result = append(result, name)
		}
	}
	return result
}

// Tags returns tag names to target commit hashes (annotated tags are peeled).
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
	return r.stashes
}

// GetTree retrieves a Tree object by its hash.
func (r *Repository) GetTree(treeHash Hash) (*Tree, error) {
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

// GetBlob retrieves raw blob data by its hash.
func (r *Repository) GetBlob(blobHash Hash) ([]byte, error) {
	objectData, objectType, err := r.readObjectData(blobHash, 0)
	if err != nil {
		return nil, fmt.Errorf("blob not found: %s", blobHash)
	}

	if objectType != packObjectBlob {
		return nil, fmt.Errorf("object %s is not a blob (type %d)", blobHash, objectType)
	}

	return objectData, nil
}

// GetCommit looks up a single commit by hash using the cached commit map.
func (r *Repository) GetCommit(hash Hash) (*Commit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if c, ok := r.commitsMap()[hash]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("commit not found: %s", hash)
}

// GetTag looks up a single tag by hash.
// This performs a linear scan over all tags.
func (r *Repository) GetTag(hash Hash) (*Tag, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.tags {
		if t.ID == hash {
			return t, nil
		}
	}
	return nil, fmt.Errorf("tag not found: %s", hash)
}

// GetCommits returns full Commit objects for the given hashes.
// Unknown hashes are silently skipped.
func (r *Repository) GetCommits(hashes []Hash) []*Commit {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cm := r.commitsMap()
	result := make([]*Commit, 0, len(hashes))
	for _, h := range hashes {
		if c, ok := cm[h]; ok {
			result = append(result, c)
		}
	}
	return result
}

// GetObjectInfo returns the object type name and size in bytes for any object.
func (r *Repository) GetObjectInfo(hash Hash) (string, int, error) {
	data, typeByte, err := r.readObjectData(hash, 0)
	if err != nil {
		return "", 0, err
	}
	return ObjectType(typeByte).String(), len(data), nil
}

// resolveTreeAtPath walks from rootTreeHash through a slash-separated dirPath
// (e.g., "internal/gitcore") and returns the tree at that location.
// Empty dirPath returns the root tree itself.
func (r *Repository) resolveTreeAtPath(rootTreeHash Hash, dirPath string) (*Tree, error) {
	if dirPath == "" || dirPath == "/" {
		return r.GetTree(rootTreeHash)
	}

	components := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentTreeHash := rootTreeHash

	for _, component := range components {
		tree, err := r.GetTree(currentTreeHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read tree %s: %w", currentTreeHash, err)
		}

		found := false
		for _, entry := range tree.Entries {
			if entry.Name == component {
				if entry.Mode != "040000" && entry.Type != objectTypeTree {
					return nil, fmt.Errorf("path component %q is not a directory", component)
				}
				currentTreeHash = entry.ID
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf("path component %q not found", component)
		}
	}

	return r.GetTree(currentTreeHash)
}
