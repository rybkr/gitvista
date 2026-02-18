package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

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

func (r *Repository) Name() string    { return filepath.Base(r.workDir) }
func (r *Repository) GitDir() string   { return r.gitDir }
func (r *Repository) WorkDir() string  { return r.workDir }

func (r *Repository) Commits() map[Hash]*Commit {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[Hash]*Commit, len(r.commits))
	for _, commit := range r.commits {
		result[commit.ID] = commit
	}
	return result
}

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

func (r *Repository) Stashes() []StashEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stashes
}

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

func (r *Repository) GetBlob(blobHash Hash) ([]byte, error) {
	objectData, objectType, err := r.readObjectData(blobHash)
	if err != nil {
		return nil, fmt.Errorf("blob not found: %s", blobHash)
	}

	if objectType != packObjectBlob {
		return nil, fmt.Errorf("object %s is not a blob (type %d)", blobHash, objectType)
	}

	return objectData, nil
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
		delta.Stashes = []StashEntry{}
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
