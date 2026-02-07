package server

import (
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

	if !delta.IsEmpty() || status != nil {
		s.broadcastUpdate(UpdateMessage{Delta: delta, Status: status})
	} else {
		log.Println("No changes detected")
	}
}
