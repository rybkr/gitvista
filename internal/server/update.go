package server

import (
	"strings"

	"github.com/rybkr/gitvista/internal/gitcore"
	"log"
)

// updateRepository reloads repository state and broadcasts changes to clients
// Called by filesystem watcher when Git operations are detected
func (s *Server) updateRepository() {
	log.Println("Updating repository...")

	s.cacheMu.RLock()
	oldRepo := s.cached.repo
	s.cacheMu.RUnlock()

	newRepo, err := gitcore.NewRepository(s.repo.GitDir())
	if err != nil {
		log.Printf("ERROR: Failed to reload repository: %v", err)
		return
	}

	var delta *gitcore.RepositoryDelta
	if oldRepo != nil {
		delta = newRepo.Diff(oldRepo)
	} else {
		// First update: treat everything as new
		delta = newRepo.Diff(&gitcore.Repository{})
	}

	s.cacheMu.Lock()
	s.repo = newRepo
	s.cached.repo = newRepo
	s.cacheMu.Unlock()

	status := getWorkingTreeStatus(newRepo.WorkDir())

	// Check if HEAD changed (detached state, branch switch, or commit)
	var headInfo *HeadInfo
	if oldRepo != nil {
		oldHead := oldRepo.Head()
		oldRef := oldRepo.HeadRef()
		oldDetached := oldRepo.HeadDetached()

		newHead := newRepo.Head()
		newRef := newRepo.HeadRef()
		newDetached := newRepo.HeadDetached()

		if oldHead != newHead || oldRef != newRef || oldDetached != newDetached {
			headInfo = buildHeadInfo(newRepo)
		}
	} else {
		// First update: always send HEAD info
		headInfo = buildHeadInfo(newRepo)
	}

	if !delta.IsEmpty() || status != nil || headInfo != nil {
		s.broadcastUpdate(UpdateMessage{Delta: delta, Status: status, Head: headInfo})
	} else {
		log.Println("No changes detected")
	}
}

// buildHeadInfo constructs a HeadInfo struct from the current repository state.
func buildHeadInfo(repo *gitcore.Repository) *HeadInfo {
	head := repo.Head()
	headRef := repo.HeadRef()
	isDetached := repo.HeadDetached()

	// Extract branch name from ref
	branchName := ""
	if headRef != "" {
		if name, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
			branchName = name
		}
	}

	// Get tag names and select recent tags (up to 5)
	tagNames := repo.TagNames()
	recentTags := tagNames
	if len(tagNames) > 5 {
		recentTags = tagNames[:5]
	}

	return &HeadInfo{
		Hash:        string(head),
		Ref:         headRef,
		BranchName:  branchName,
		IsDetached:  isDetached,
		CommitCount: len(repo.Commits()),
		BranchCount: len(repo.Branches()),
		TagCount:    len(tagNames),
		Description: repo.Description(),
		Remotes:     repo.Remotes(),
		RecentTags:  recentTags,
	}
}
