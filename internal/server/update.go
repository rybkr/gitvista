package server

import (
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// updateRepository reloads repository state and broadcasts changes to clients.
func (s *Server) updateRepository() {
	s.logger.Debug("Updating repository")

	s.cacheMu.RLock()
	oldRepo := s.cached.repo
	gitDir := oldRepo.GitDir()
	s.cacheMu.RUnlock()

	newRepo, err := gitcore.NewRepository(gitDir)
	if err != nil {
		s.logger.Error("Failed to reload repository", "err", err)
		return
	}

	var delta *gitcore.RepositoryDelta
	if oldRepo != nil {
		delta = newRepo.Diff(oldRepo)
	} else {
		delta = newRepo.Diff(&gitcore.Repository{})
	}

	s.cacheMu.Lock()
	s.cached.repo = newRepo
	s.cacheMu.Unlock()

	status := getWorkingTreeStatus(newRepo.WorkDir())

	var headInfo *HeadInfo
	headChanged := oldRepo == nil ||
		oldRepo.Head() != newRepo.Head() ||
		oldRepo.HeadRef() != newRepo.HeadRef() ||
		oldRepo.HeadDetached() != newRepo.HeadDetached()
	if headChanged {
		headInfo = buildHeadInfo(newRepo)
	}

	if !delta.IsEmpty() || status != nil || headInfo != nil {
		s.broadcastUpdate(UpdateMessage{Delta: delta, Status: status, Head: headInfo})
	} else {
		s.logger.Debug("No changes detected after repository reload")
	}
}

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
		CommitCount: len(repo.Commits()),
		BranchCount: len(repo.Branches()),
		TagCount:    len(tagNames),
		Description: repo.Description(),
		Remotes:     repo.Remotes(),
		RecentTags:  recentTags,
	}
}
