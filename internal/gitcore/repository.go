// Package gitcore provides pure Go implementation of Git object parsing and repository traversal.
package gitcore

import (
	"fmt"
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
