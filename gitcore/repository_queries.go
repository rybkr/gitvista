package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
		result[hash] = cloneCommit(commit)
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
	for i, stash := range r.stashes {
		result[i] = cloneStashEntry(stash)
	}
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

// Description returns the .git/description contents, or empty string if the file
// is missing or contains Git's default placeholder text.
func (r *Repository) Description() string {
	descPath := filepath.Join(r.gitDir, "description")
	// #nosec G304 -- description path is controlled by repository location
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

// TagNames returns all tag names in the repository.
func (r *Repository) TagNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0)
	for ref := range r.refs {
		if name, ok := strings.CutPrefix(ref, "refs/tags/"); ok {
			result = append(result, name)
		}
	}
	slices.Sort(result)
	return result
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

// GetTree retrieves a tree object by its hash.
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

	if objectType != ObjectTypeBlob {
		return nil, fmt.Errorf("object %s is not a blob (type %d)", blobHash, objectType)
	}

	return objectData, nil
}

// GetCommit looks up a single commit by hash using the cached commit map.
func (r *Repository) GetCommit(hash Hash) (*Commit, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if commit, ok := r.commitMap[hash]; ok {
		return cloneCommit(commit), nil
	}
	return nil, fmt.Errorf("commit not found: %s", hash)
}

func cloneCommit(commit *Commit) *Commit {
	if commit == nil {
		return nil
	}

	cloned := *commit
	if commit.Parents != nil {
		cloned.Parents = append([]Hash(nil), commit.Parents...)
	}
	return &cloned
}

func cloneStashEntry(stash *StashEntry) *StashEntry {
	if stash == nil {
		return nil
	}

	cloned := *stash
	return &cloned
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
				if entry.Mode != "040000" && entry.Type != ObjectTypeTree {
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

func parseRemotesFromConfig(config string) map[string]string {
	remotes := make(map[string]string)
	var currentRemote string

	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "[remote \"") && strings.HasSuffix(line, "\"]") {
			start := strings.Index(line, "\"") + 1
			end := strings.LastIndex(line, "\"")
			if start > 0 && end > start {
				currentRemote = line[start:end]
			}
			continue
		}

		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[remote") {
			currentRemote = ""
			continue
		}

		if currentRemote != "" && strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			remotes[currentRemote] = stripCredentials(url)
			currentRemote = ""
		}
	}

	return remotes
}

func stripCredentials(url string) string {
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(url, scheme) && strings.Contains(url, "@") {
			parts := strings.SplitN(url, "@", 2)
			if len(parts) == 2 {
				return scheme + parts[1]
			}
		}
	}
	return url
}
