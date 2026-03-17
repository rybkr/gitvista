package repositoryview

import "github.com/rybkr/gitvista/internal/gitcore"

type RepositoryDelta struct {
	AddedCommits   []*gitcore.Commit `json:"addedCommits"`
	DeletedCommits []*gitcore.Commit `json:"deletedCommits"`

	AddedBranches   map[string]gitcore.Hash `json:"addedBranches"`
	AmendedBranches map[string]gitcore.Hash `json:"amendedBranches"`
	DeletedBranches map[string]gitcore.Hash `json:"deletedBranches"`

	HeadHash string               `json:"headHash"`
	Tags     map[string]string    `json:"tags"`
	Stashes  []*gitcore.StashEntry `json:"stashes"`

	Bootstrap         bool `json:"bootstrap,omitempty"`
	BootstrapComplete bool `json:"bootstrapComplete,omitempty"`
}

func NewRepositoryDelta() *RepositoryDelta {
	return &RepositoryDelta{
		AddedBranches:   make(map[string]gitcore.Hash),
		AmendedBranches: make(map[string]gitcore.Hash),
		DeletedBranches: make(map[string]gitcore.Hash),
		Tags:            make(map[string]string),
		Stashes:         make([]*gitcore.StashEntry, 0),
	}
}

func (d *RepositoryDelta) IsEmpty() bool {
	return len(d.AddedCommits) == 0 &&
		len(d.DeletedCommits) == 0 &&
		len(d.AddedBranches) == 0 &&
		len(d.DeletedBranches) == 0 &&
		len(d.AmendedBranches) == 0
}

func DiffRepositories(newRepo, oldRepo *gitcore.Repository) *RepositoryDelta {
	if newRepo == nil {
		return NewRepositoryDelta()
	}
	if oldRepo == nil {
		oldRepo = gitcore.NewEmptyRepository()
	}

	delta := NewRepositoryDelta()
	newAttribution := commitBranchAttribution(newRepo)
	oldAttribution := commitBranchAttribution(oldRepo)

	newCommits := newRepo.Commits()
	oldCommits := oldRepo.Commits()
	for hash, commit := range newCommits {
		if _, found := oldCommits[hash]; !found {
			delta.AddedCommits = append(delta.AddedCommits, cloneCommitWithBranchAttribution(commit, newAttribution[hash]))
		}
	}
	for hash, commit := range oldCommits {
		if _, found := newCommits[hash]; !found {
			delta.DeletedCommits = append(delta.DeletedCommits, cloneCommitWithBranchAttribution(commit, oldAttribution[hash]))
		}
	}

	newBranches := newRepo.GraphBranches()
	oldBranches := oldRepo.GraphBranches()
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

	delta.HeadHash = string(newRepo.Head())
	delta.Tags = newRepo.Tags()
	delta.Stashes = newRepo.Stashes()
	if delta.Stashes == nil {
		delta.Stashes = make([]*gitcore.StashEntry, 0)
	}

	return delta
}
