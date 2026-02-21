package repomanager

import (
	"fmt"
	"os"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

// fetchLoop periodically fetches updates for all ready repos.
func (rm *RepoManager) fetchLoop() {
	defer rm.wg.Done()

	ticker := time.NewTicker(rm.cfg.FetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.fetchAll()
		}
	}
}

// fetchAll fetches updates for all repos in StateReady.
func (rm *RepoManager) fetchAll() {
	rm.mu.RLock()
	var ready []*ManagedRepo
	for _, managed := range rm.repos {
		managed.mu.RLock()
		if managed.State == StateReady {
			ready = append(ready, managed)
		}
		managed.mu.RUnlock()
	}
	rm.mu.RUnlock()

	for _, managed := range ready {
		managed.mu.RLock()
		diskPath := managed.DiskPath
		managed.mu.RUnlock()

		if err := fetchRepo(rm.ctx, diskPath, rm.cfg.FetchTimeout); err != nil {
			rm.logger.Warn("fetch failed", "id", managed.ID, "error", err)
			continue
		}

		// Reload the repository to pick up new objects
		repo, err := gitcore.NewRepository(diskPath)
		if err != nil {
			rm.logger.Warn("repo reload failed after fetch", "id", managed.ID, "error", err)
			continue
		}

		managed.mu.Lock()
		managed.Repo = repo
		managed.LastFetch = time.Now()
		managed.mu.Unlock()

		rm.logger.Debug("fetch completed", "id", managed.ID)
	}
}

// evictionLoop periodically removes repos that have been inactive.
func (rm *RepoManager) evictionLoop() {
	defer rm.wg.Done()

	interval := max(rm.cfg.InactivityTTL/10, time.Minute)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.evictInactive()
		}
	}
}

// evictInactive removes repos that haven't been accessed within InactivityTTL.
func (rm *RepoManager) evictInactive() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	now := time.Now()
	var toEvict []string

	for id, managed := range rm.repos {
		managed.mu.RLock()
		state := managed.State
		lastAccess := managed.LastAccess
		managed.mu.RUnlock()

		// Skip repos that are being cloned or pending
		if state == StatePending || state == StateCloning {
			continue
		}

		if now.Sub(lastAccess) > rm.cfg.InactivityTTL {
			toEvict = append(toEvict, id)
		}
	}

	for _, id := range toEvict {
		managed := rm.repos[id]
		managed.mu.RLock()
		diskPath := managed.DiskPath
		managed.mu.RUnlock()

		if err := os.RemoveAll(diskPath); err != nil {
			rm.logger.Warn("failed to remove evicted repo", "id", id, "path", diskPath, "error", err)
		}

		delete(rm.repos, id)
		rm.logger.Info("evicted inactive repo", "id", id,
			"inactive_for", fmt.Sprintf("%s", now.Sub(managed.LastAccess)))
	}
}
