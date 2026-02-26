package gitcore

import (
	"container/heap"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Repository represents a Git repository, providing access to its commits,
// branches, tags, and other metadata.
type Repository struct {
	gitDir  string
	workDir string

	refs        map[string]Hash
	commits     []*Commit
	commitMap   map[Hash]*Commit
	tags        []*Tag
	stashes     []*StashEntry
	packIndices []*PackIndex
	mailmap     *Mailmap

	head         Hash
	headRef      string
	headDetached bool

	mu sync.RWMutex
}

// NewEmptyRepository returns a Repository with all maps initialized but
// containing no data. Used as the "old" state when computing the initial delta.
func NewEmptyRepository() *Repository {
	return &Repository{
		refs:      make(map[string]Hash),
		commits:   make([]*Commit, 0),
		commitMap: make(map[Hash]*Commit),
		tags:      make([]*Tag, 0),
		stashes:   make([]*StashEntry, 0),
	}
}

// NewRepository opens a Git repository starting from path, which can be
// the working directory, the .git directory, or any parent directory.
func NewRepository(path string) (*Repository, error) {
	gitDir, workDir, err := findGitDirectory(path)
	if err != nil {
		return nil, err
	}
	if err := validateGitDirectory(gitDir); err != nil {
		return nil, err
	}

	repo := &Repository{
		gitDir:      gitDir,
		workDir:     workDir,
		refs:        make(map[string]Hash),
		commits:     make([]*Commit, 0),
		tags:        make([]*Tag, 0),
		stashes:     make([]*StashEntry, 0),
		packIndices: make([]*PackIndex, 0),
	}

	if err := repo.loadPackIndices(); err != nil {
		return nil, fmt.Errorf("failed to load pack indices: %w", err)
	}
	if err := repo.loadRefs(); err != nil {
		return nil, fmt.Errorf("failed to load refs: %w", err)
	}
	if err := repo.loadStashes(); err != nil {
		return nil, fmt.Errorf("failed to load stashes: %w", err)
	}
	if err := repo.loadObjects(); err != nil {
		return nil, fmt.Errorf("failed to load objects: %w", err)
	}
	if err := repo.loadMailmap(); err != nil {
		return nil, fmt.Errorf("failed to load mailmap: %w", err)
	}

	return repo, nil
}

// Name returns the base name of the repository's working directory.
func (r *Repository) Name() string { return filepath.Base(r.workDir) }

// GitDir returns the path to the repository's .git directory.
func (r *Repository) GitDir() string { return r.gitDir }

// WorkDir returns the path to the repository's working directory.
func (r *Repository) WorkDir() string { return r.workDir }

// IsBare reports whether the repository is a bare repository.
func (r *Repository) IsBare() bool { return r.gitDir == r.workDir }

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
		panic("gitcore: commitMap is nil — Repository was not fully initialized via NewRepository")
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
	//nolint:gosec // G304: Description path is controlled by git repository structure
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
	//nolint:gosec // G304: Config path is controlled by git repository structure
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

// BuildGraphSummary constructs a lightweight GraphSummary containing only the
// topology (parent hashes) and temporal data (committer timestamps) for every
// commit, plus branches, tags, HEAD, and stashes. This is ~7-8x smaller than
// the full commit set and enables the client to compute graph layout without
// materializing heavyweight commit data.
func (r *Repository) BuildGraphSummary() *GraphSummary {
	// Build skeletons and read head under the lock, then release before
	// calling Branches/Tags/Stashes (which acquire their own RLock).
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
		Branches:        r.Branches(),
		Tags:            r.Tags(),
		HeadHash:        headHash,
		Stashes:         r.Stashes(),
		OldestTimestamp: oldest,
		NewestTimestamp: newest,
	}
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
				if entry.Mode != "040000" && entry.Type != "tree" {
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

// findGitDirectory walks up from startPath to locate the .git directory.
func findGitDirectory(startPath string) (gitDir string, workDir string, err error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if filepath.Base(absPath) == ".git" {
		info, err := os.Stat(absPath)
		if err == nil && info.IsDir() {
			return absPath, filepath.Dir(absPath), nil
		}
	}

	if isBareRepository(absPath) {
		return absPath, absPath, nil
	}

	currentPath := absPath
	for {
		gitPath := filepath.Join(currentPath, ".git")

		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return gitPath, currentPath, nil
			}
			return handleGitFile(gitPath, currentPath)
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return "", "", fmt.Errorf("not a git repository (or any parent up to mount point): %s", startPath)
		}
		currentPath = parentPath
	}
}

// handleGitFile handles .git files (worktrees, submodules) with format "gitdir: <path>".
func handleGitFile(gitFilePath string, workDir string) (string, string, error) {
	//nolint:gosec // G304: .git file path is controlled by repository location
	content, err := os.ReadFile(gitFilePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read .git file: %w", err)
	}

	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", "", fmt.Errorf("invalid .git file format: %s", gitFilePath)
	}

	gitDir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(filepath.Dir(gitFilePath), gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	if _, err := os.Stat(gitDir); err != nil {
		return "", "", fmt.Errorf("gitdir points to non-existent directory: %s", gitDir)
	}

	return gitDir, workDir, nil
}

// validateGitDirectory checks that gitDir exists, is a directory, and contains
// the expected Git internals (objects, refs, HEAD).
func validateGitDirectory(gitDir string) error {
	info, err := os.Stat(gitDir)
	if err != nil {
		return fmt.Errorf("git directory does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("git path is not a directory: %s", gitDir)
	}

	requiredPaths := []string{"objects", "refs", "HEAD"}
	for _, required := range requiredPaths {
		path := filepath.Join(gitDir, required)
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("invalid git repository, missing: %s", required)
		}
	}

	return nil
}

// isBareRepository checks whether path looks like a bare Git repository.
// A bare repo is a directory containing objects/, refs/, and HEAD but no .git subdirectory.
func isBareRepository(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		return false
	}
	for _, required := range []string{"objects", "refs", "HEAD"} {
		if _, err := os.Stat(filepath.Join(path, required)); err != nil {
			return false
		}
	}
	return true
}

// parseRemotesFromConfig parses a Git config file and returns a map of remote
// names to their URLs, with credentials stripped.
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
			currentRemote = "" // Only capture first URL per remote
		}
	}

	return remotes
}

// stripCredentials removes embedded credentials from HTTP/HTTPS URLs,
// returning the URL with only the host and path portions intact.
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
