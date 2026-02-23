package server

import (
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// updateRepository has been moved to RepoSession in session.go.

func buildHeadInfo(repo *gitcore.Repository) *HeadInfo {
	headRef := repo.HeadRef()

	branchName := ""
	if headRef != "" {
		if name, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
			branchName = name
		}
	}

	tagNames := repo.TagNames()
	recentTags := tagNames
	if len(tagNames) > 5 {
		recentTags = tagNames[:5]
	}

	return &HeadInfo{
		Hash:        string(repo.Head()),
		Ref:         headRef,
		BranchName:  branchName,
		IsDetached:  repo.HeadDetached(),
		CommitCount: repo.CommitCount(),
		BranchCount: len(repo.Branches()),
		TagCount:    len(tagNames),
		Description: repo.Description(),
		Remotes:     repo.Remotes(),
		RecentTags:  recentTags,
	}
}
