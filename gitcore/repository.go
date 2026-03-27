package gitcore

import (
	"fmt"
	"runtime"
	"sync"
)

// Repository represents a Git repository, providing access to
// its commits, branches, tags, analytics, and other metadata.
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
	mailmap       *Mailmap

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
	if err := repo.loadMailmap(); err != nil {
		return nil, fmt.Errorf("failed to load mailmap: %w", err)
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
			if err := unmapPackData(reader.data); err != nil && closeErr == nil {
				closeErr = fmt.Errorf("unmap pack file %s: %w", path, err)
			}
			if err := reader.file.Close(); err != nil && closeErr == nil {
				closeErr = fmt.Errorf("close pack file %s: %w", path, err)
			}
		}
		r.packReaders = nil
	})
	return closeErr
}
