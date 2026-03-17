package repositoryview

import "github.com/rybkr/gitvista/gitcore"

type RepositoryDelta struct {
	AddedCommits   []*gitcore.Commit `json:"addedCommits"`
	DeletedCommits []*gitcore.Commit `json:"deletedCommits"`

	AddedBranches   map[string]gitcore.Hash `json:"addedBranches"`
	AmendedBranches map[string]gitcore.Hash `json:"amendedBranches"`
	DeletedBranches map[string]gitcore.Hash `json:"deletedBranches"`

	HeadHash string                `json:"headHash"`
	Tags     map[string]string     `json:"tags"`
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

	return diffRepositories(
		newRepo.Commits(),
		oldRepo.Commits(),
		commitBranchAttribution(newRepo),
		commitBranchAttribution(oldRepo),
		newRepo.GraphBranches(),
		oldRepo.GraphBranches(),
		newRepo.Head(),
		newRepo.Tags(),
		newRepo.Stashes(),
	)
}

func diffRepositories(
	newCommits map[gitcore.Hash]*gitcore.Commit,
	oldCommits map[gitcore.Hash]*gitcore.Commit,
	newAttribution map[gitcore.Hash]branchAttribution,
	oldAttribution map[gitcore.Hash]branchAttribution,
	newBranches map[string]gitcore.Hash,
	oldBranches map[string]gitcore.Hash,
	head gitcore.Hash,
	tags map[string]string,
	stashes []*gitcore.StashEntry,
) *RepositoryDelta {
	delta := NewRepositoryDelta()
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

	delta.HeadHash = string(head)
	delta.Tags = tags
	delta.Stashes = stashes
	if delta.Stashes == nil {
		delta.Stashes = make([]*gitcore.StashEntry, 0)
	}

	return delta
}
