package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Repository represents a Git repository with its metadata and object storage.
type Repository struct {
	gitDir  string
	workDir string

	packIndices []*PackIndex
	refs        map[string]Hash
	commits     []*Commit
	tags        []*Tag
	stashes     []StashEntry

	head         Hash
	headRef      string
	headDetached bool

	mu sync.RWMutex
}

// NewRepository creates and initializes a new Repository instance.
// path can be either:
//   - The working directory (will find .git within)
//   - The .git directory itself
//   - A parent directory containing a .git directory
func NewRepository(path string) (*Repository, error) {
	gitDir, workDir, err := findGitDirectory(path)
	if err != nil {
		return nil, err
	}

	if err := validateGitDirectory(gitDir); err != nil {
		return nil, err
	}

	repo := &Repository{
		gitDir:  gitDir,
		workDir: workDir,
		refs:    make(map[string]Hash),
		commits: make([]*Commit, 0),
	}

	if err := repo.loadPackIndices(); err != nil {
		return nil, fmt.Errorf("failed to load pack indices: %w", err)
	}
	if err := repo.loadRefs(); err != nil {
		return nil, fmt.Errorf("failed to load refs: %w", err)
	}
	repo.loadObjects()
	repo.stashes = repo.loadStashes()

	return repo, nil
}

// Name returns the repository's directory name.
func (r *Repository) Name() string {
	return filepath.Base(r.workDir)
}

// GitDir returns the path to the repository's .git folder.
func (r *Repository) GitDir() string {
	return r.gitDir
}

// WorkDir returns the path to the repository's working directory.
func (r *Repository) WorkDir() string {
	return r.workDir
}

// Commits returns a map of all commit IDs to Commit structs.
func (r *Repository) Commits() map[Hash]*Commit {
	result := make(map[Hash]*Commit)
	for _, commit := range r.commits {
		result[commit.ID] = commit
	}
	return result
}

// Branches returns a map of all branch names to Commit hashes.
func (r *Repository) Branches() map[string]Hash {
	result := make(map[string]Hash)
	for ref, hash := range r.refs {
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			result[name] = hash
		}
	}
	return result
}

// Head returns the current HEAD commit hash.
func (r *Repository) Head() Hash {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.head
}

// HeadRef returns the current HEAD ref (e.g., "refs/heads/main").
// Empty string if HEAD is detached.
func (r *Repository) HeadRef() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headRef
}

// HeadDetached returns true if HEAD is detached (not pointing to a branch).
func (r *Repository) HeadDetached() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.headDetached
}

// Description returns the repository description from .git/description.
// Returns empty string if the file doesn't exist or contains the default placeholder.
func (r *Repository) Description() string {
	descPath := filepath.Join(r.gitDir, "description")
	//nolint:gosec // G304: Description path is controlled by git repository structure
	content, err := os.ReadFile(descPath)
	if err != nil {
		return ""
	}

	desc := strings.TrimSpace(string(content))

	// Filter out Git's default placeholder text
	if desc == "Unnamed repository; edit this file 'description' to name the repository." {
		return ""
	}

	return desc
}

// Remotes returns a map of remote names to their URLs by parsing .git/config.
// Credentials are stripped from HTTPS URLs before returning.
func (r *Repository) Remotes() map[string]string {
	configPath := filepath.Join(r.gitDir, "config")
	//nolint:gosec // G304: Config path is controlled by git repository structure
	content, err := os.ReadFile(configPath)
	if err != nil {
		return make(map[string]string)
	}

	return parseRemotesFromConfig(string(content))
}

// TagNames returns a slice of all tag names.
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

// Tags returns a map of tag names to their target commit hashes.
// Annotated tags are peeled to the commit they point at.
func (r *Repository) Tags() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Build lookup: annotated tag object hash -> commit hash
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
		// Peel annotated tag objects to their commit target.
		if commitHash, isAnnotated := annotatedTargets[hash]; isAnnotated {
			result[name] = string(commitHash)
		} else {
			result[name] = string(hash)
		}
	}
	return result
}

// Stashes returns all stash entries for this repository, newest first.
func (r *Repository) Stashes() []StashEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stashes
}

// GetTree loads and returns a tree object by its hash.
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

// GetBlob loads and returns raw blob content by its hash.
func (r *Repository) GetBlob(blobHash Hash) ([]byte, error) {
	// readObjectData handles both loose and packed objects, returning raw data and type byte.
	objectData, objectType, err := r.readObjectData(blobHash)
	if err != nil {
		return nil, fmt.Errorf("blob not found: %s", blobHash)
	}

	// Type 3 = blob (see objects.go line 149)
	if objectType != 3 {
		return nil, fmt.Errorf("object %s is not a blob (type %d)", blobHash, objectType)
	}

	return objectData, nil
}

// resolveTreeAtPath navigates from a root tree hash to a tree at the given directory path.
// dirPath is slash-separated (e.g., "internal/gitcore"). Empty string returns the root tree.
// Returns nil and an error if the path does not exist or is not a tree.
func (r *Repository) resolveTreeAtPath(rootTreeHash Hash, dirPath string) (*Tree, error) {
	// Empty path means the root tree itself
	if dirPath == "" || dirPath == "/" {
		return r.GetTree(rootTreeHash)
	}

	// Split path by '/' and walk through each component
	components := strings.Split(strings.Trim(dirPath, "/"), "/")
	currentTreeHash := rootTreeHash

	for _, component := range components {
		tree, err := r.GetTree(currentTreeHash)
		if err != nil {
			return nil, fmt.Errorf("failed to read tree %s: %w", currentTreeHash, err)
		}

		// Find the entry matching this path component
		found := false
		for _, entry := range tree.Entries {
			if entry.Name == component {
				// Must be a tree (directory)
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

	// Return the final tree
	return r.GetTree(currentTreeHash)
}

// Diff returns the difference between this repository and another,
// represented as a RepositoryDelta struct.
// It treats r as the new repository and old as the old repository.
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

	// Always include current HEAD, tags, and stashes so the frontend stays in sync.
	delta.HeadHash = string(r.Head())
	delta.Tags = r.Tags()
	delta.Stashes = r.Stashes()
	if delta.Stashes == nil {
		delta.Stashes = []StashEntry{}
	}

	return delta
}

// findGitDirectory locates the .git directory starting from the given path.
// Returns both the .git directory and the working directory.
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

// handleGitFile handles the case where .git is a file (worktrees, submodules).
// .git file format: "gitdir: /path/to/actual/.git".
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

// validateGitDirectory checks if the directory is a valid Git repository.
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

// parseRemotesFromConfig parses [remote "name"] sections from .git/config.
// Returns a map of remote names to their URLs with credentials stripped.
func parseRemotesFromConfig(config string) map[string]string {
	remotes := make(map[string]string)
	var currentRemote string

	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)

		// Match [remote "origin"] section headers
		if strings.HasPrefix(line, "[remote \"") && strings.HasSuffix(line, "\"]") {
			start := strings.Index(line, "\"") + 1
			end := strings.LastIndex(line, "\"")
			if start > 0 && end > start {
				currentRemote = line[start:end]
			}
			continue
		}

		// Reset current remote when entering a different section
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[remote") {
			currentRemote = ""
			continue
		}

		// Parse url = ... within remote section
		if currentRemote != "" && strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			remotes[currentRemote] = stripCredentials(url)
			currentRemote = "" // Only capture first URL per remote
		}
	}

	return remotes
}

// stripCredentials removes username:password from HTTPS URLs.
func stripCredentials(url string) string {
	// Match https://username:password@host/path
	if strings.HasPrefix(url, "https://") && strings.Contains(url, "@") {
		parts := strings.SplitN(url, "@", 2)
		if len(parts) == 2 {
			return "https://" + parts[1]
		}
	}
	return url
}
