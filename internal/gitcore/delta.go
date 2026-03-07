package gitcore

// RepositoryDelta is the wire format sent to the frontend during live updates.
type RepositoryDelta struct {
	AddedCommits   []*Commit `json:"addedCommits"`
	DeletedCommits []*Commit `json:"deletedCommits"`

	AddedBranches   map[string]Hash `json:"addedBranches"`
	AmendedBranches map[string]Hash `json:"amendedBranches"`
	DeletedBranches map[string]Hash `json:"deletedBranches"`

	// HeadHash, Tags, and Stashes are sent on every delta so the frontend stays in sync.
	HeadHash string            `json:"headHash"`
	Tags     map[string]string `json:"tags"` // tag name -> target commit hash (annotated tags are peeled)
	Stashes  []*StashEntry     `json:"stashes"`

	// Bootstrap indicates this delta is part of initial history bootstrap.
	// BootstrapComplete is true on the final bootstrap batch.
	Bootstrap         bool `json:"bootstrap,omitempty"`
	BootstrapComplete bool `json:"bootstrapComplete,omitempty"`
}

// NewRepositoryDelta creates a RepositoryDelta with all maps and slices initialized.
func NewRepositoryDelta() *RepositoryDelta {
	return &RepositoryDelta{
		AddedBranches:   make(map[string]Hash),
		AmendedBranches: make(map[string]Hash),
		DeletedBranches: make(map[string]Hash),
		Tags:            make(map[string]string),
		Stashes:         make([]*StashEntry, 0),
	}
}

// IsEmpty reports whether the delta contains no changes.
func (d *RepositoryDelta) IsEmpty() bool {
	return len(d.AddedCommits) == 0 &&
		len(d.DeletedCommits) == 0 &&
		len(d.AddedBranches) == 0 &&
		len(d.DeletedBranches) == 0 &&
		len(d.AmendedBranches) == 0
}
