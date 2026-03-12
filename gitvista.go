package gitvista

import "github.com/rybkr/gitvista/internal/gitcore"

// Repository is the public module-root wrapper around the internal Git engine.
// It exposes a stable entrypoint without leaking gitcore into import paths.
type Repository struct {
	core *gitcore.Repository
}

type (
	Hash              = gitcore.Hash
	Commit            = gitcore.Commit
	Tag               = gitcore.Tag
	StashEntry        = gitcore.StashEntry
	GraphSummary      = gitcore.GraphSummary
	CommitSkeleton    = gitcore.CommitSkeleton
	WorkingTreeStatus = gitcore.WorkingTreeStatus
	FileStatus        = gitcore.FileStatus
	UpstreamTracking  = gitcore.UpstreamTracking
)

// Open opens a Git repository from a working tree, .git directory, or parent path.
func Open(path string) (*Repository, error) {
	repo, err := gitcore.NewRepository(path)
	if err != nil {
		return nil, err
	}
	return &Repository{core: repo}, nil
}

// OpenRepository is a compatibility alias for Open.
func OpenRepository(path string) (*Repository, error) {
	return Open(path)
}

// Close releases resources owned by the repository.
func (r *Repository) Close() error {
	if r == nil || r.core == nil {
		return nil
	}
	return r.core.Close()
}

// Name returns the repository name.
func (r *Repository) Name() string {
	return r.core.Name()
}

// GitDir returns the repository's Git directory path.
func (r *Repository) GitDir() string {
	return r.core.GitDir()
}

// WorkDir returns the repository's working tree path.
func (r *Repository) WorkDir() string {
	return r.core.WorkDir()
}

// Head returns the current HEAD commit hash.
func (r *Repository) Head() Hash {
	return r.core.Head()
}

// Branches returns local branches keyed by short branch name.
func (r *Repository) Branches() map[string]Hash {
	return r.core.Branches()
}

// Tags returns tag names mapped to their target commit hash.
func (r *Repository) Tags() map[string]string {
	return r.core.Tags()
}

// Remotes returns remote names mapped to sanitized URLs.
func (r *Repository) Remotes() map[string]string {
	return r.core.Remotes()
}

// CurrentBranchUpstream returns tracking information for the current branch, if any.
func (r *Repository) CurrentBranchUpstream() *UpstreamTracking {
	return r.core.CurrentBranchUpstream()
}

// GraphSummary returns a lightweight summary of commit topology and refs.
func (r *Repository) GraphSummary() *GraphSummary {
	return r.core.BuildGraphSummary()
}

// WorkingTreeStatus computes staged, unstaged, and untracked file state.
func (r *Repository) WorkingTreeStatus() (*WorkingTreeStatus, error) {
	return gitcore.ComputeWorkingTreeStatus(r.core)
}
