package repositoryview

import (
	"slices"

	"github.com/rybkr/gitvista/internal/gitcore"
)

type CommitSkeleton struct {
	Hash              gitcore.Hash   `json:"h"`
	Parents           []gitcore.Hash `json:"p,omitempty"`
	Timestamp         int64          `json:"t"`
	BranchLabel       string         `json:"branchLabel,omitempty"`
	BranchLabelSource string         `json:"branchLabelSource,omitempty"`
}

type GraphSummary struct {
	TotalCommits    int                     `json:"totalCommits"`
	Skeleton        []CommitSkeleton        `json:"skeleton"`
	Branches        map[string]gitcore.Hash `json:"branches"`
	Tags            map[string]string       `json:"tags"`
	HeadHash        string                  `json:"headHash"`
	Stashes         []*gitcore.StashEntry   `json:"stashes"`
	OldestTimestamp int64                   `json:"oldestTimestamp"`
	NewestTimestamp int64                   `json:"newestTimestamp"`
}

func BuildGraphSummary(repo *gitcore.Repository) *GraphSummary {
	if repo == nil {
		return &GraphSummary{}
	}

	attribution := commitBranchAttribution(repo)
	commitsMap := repo.Commits()
	commits := make([]*gitcore.Commit, 0, len(commitsMap))
	for _, commit := range commitsMap {
		commits = append(commits, commit)
	}
	slices.SortFunc(commits, func(a, b *gitcore.Commit) int {
		if a == nil || b == nil {
			if a == nil && b == nil {
				return 0
			}
			if a == nil {
				return 1
			}
			return -1
		}
		if a.Committer.When.Equal(b.Committer.When) {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		}
		if a.Committer.When.Before(b.Committer.When) {
			return -1
		}
		return 1
	})

	skeletons := make([]CommitSkeleton, 0, len(commits))
	var oldest, newest int64
	for _, commit := range commits {
		if commit == nil {
			continue
		}
		ts := commit.Committer.When.Unix()
		attr := attribution[commit.ID]
		skeletons = append(skeletons, CommitSkeleton{
			Hash:              commit.ID,
			Parents:           append([]gitcore.Hash(nil), commit.Parents...),
			Timestamp:         ts,
			BranchLabel:       attr.Label,
			BranchLabelSource: attr.Source,
		})
		if oldest == 0 || ts < oldest {
			oldest = ts
		}
		if ts > newest {
			newest = ts
		}
	}

	return &GraphSummary{
		TotalCommits:    len(commits),
		Skeleton:        skeletons,
		Branches:        repo.GraphBranches(),
		Tags:            repo.Tags(),
		HeadHash:        string(repo.Head()),
		Stashes:         repo.Stashes(),
		OldestTimestamp: oldest,
		NewestTimestamp: newest,
	}
}

func AttributedCommits(repo *gitcore.Repository, hashes []gitcore.Hash) []*gitcore.Commit {
	if repo == nil {
		return nil
	}

	attribution := commitBranchAttribution(repo)
	commits := repo.Commits()
	result := make([]*gitcore.Commit, 0, len(hashes))
	for _, hash := range hashes {
		if commit, ok := commits[hash]; ok {
			result = append(result, cloneCommitWithBranchAttribution(commit, attribution[hash]))
		}
	}
	return result
}
