package gitcore

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Repository represents a Git repository, providing access to its commits,
// branches, tags, analytics, and other metadata.
type Repository struct {
	gitDir  string
	workDir string

	refs          map[string]Hash
	commits       []*Commit
	commitMap     map[Hash]*Commit
	tags          []*Tag
	stashes       []*StashEntry
	packIndices   []*PackIndex
	packLocations map[Hash]PackLocation

	head         Hash
	headRef      string
	headDetached bool

	mu sync.RWMutex

	packReadersMu sync.Mutex
	packReaders   map[string]*PackReader
	closeOnce     sync.Once
}

// NewRepository opens a Git repository starting from path, which can be the
// working directory, the .git directory, or any child directory.
func NewRepository(path string) (*Repository, error) {
	gitDir, workDir, err := findGitDirectory(path)
	if err != nil {
		return nil, err
	}
	if err := validateGitDirectory(gitDir); err != nil {
		return nil, err
	}

	repo := &Repository{
		gitDir:        gitDir,
		workDir:       workDir,
		refs:          make(map[string]Hash),
		commits:       make([]*Commit, 0),
		commitMap:     make(map[Hash]*Commit),
		tags:          make([]*Tag, 0),
		stashes:       make([]*StashEntry, 0),
		packIndices:   make([]*PackIndex, 0),
		packLocations: make(map[Hash]PackLocation),
		packReaders:   make(map[string]*PackReader),
	}
	runtime.SetFinalizer(repo, func(r *Repository) {
		_ = r.Close()
	})

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

	return repo, nil
}

// NewEmptyRepository returns a Repository with all maps initialized but
// containing no data. Used as the "old" state when computing the initial delta.
func NewEmptyRepository() *Repository {
	return &Repository{
		refs:          make(map[string]Hash),
		commits:       make([]*Commit, 0),
		commitMap:     make(map[Hash]*Commit),
		tags:          make([]*Tag, 0),
		stashes:       make([]*StashEntry, 0),
		packLocations: make(map[Hash]PackLocation),
		packReaders:   make(map[string]*PackReader),
	}
}

// Close releases repository-owned resources such as cached pack file handles.
func (r *Repository) Close() error {
	var closeErr error
	r.closeOnce.Do(func() {
		runtime.SetFinalizer(r, nil)

		r.packReadersMu.Lock()
		defer r.packReadersMu.Unlock()

		for path, reader := range r.packReaders {
			if err := reader.file.Close(); err != nil && closeErr == nil {
				closeErr = fmt.Errorf("close pack file %s: %w", path, err)
			}
		}
		r.packReaders = nil
	})
	return closeErr
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
