package server

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceTime = 100 * time.Millisecond

// statusPollInterval controls how often the working tree is polled for
// changes that do not touch .git (e.g., new untracked files, edits).
const statusPollInterval = 2 * time.Second

func (s *Server) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	repo := s.localSession.Repo()
	gitDir := repo.GitDir()
	if err := watcher.Add(gitDir); err != nil {
		return err
	}

	// fsnotify does not recurse into subdirectories. We must explicitly
	// watch refs/heads, refs/tags, and refs/remotes so that branch and tag
	// creation/deletion events (which touch files inside those dirs) are
	// picked up. walkAndWatch also handles hierarchical branch names
	// (e.g., refs/heads/feature/login) by walking the entire subtree.
	for _, sub := range []string{"refs/heads", "refs/tags", "refs/remotes"} {
		dir := filepath.Join(gitDir, sub)
		walkAndWatch(watcher, dir, s.logger)
	}

	s.wg.Add(1)
	go s.statusPollLoop()

	go s.watchLoop(watcher)

	s.logger.Info("Watching Git repository for changes", "gitDir", gitDir)
	return nil
}

// walkAndWatch adds fsnotify watches to dir and all its subdirectories.
// Missing directories are silently skipped.
func walkAndWatch(watcher *fsnotify.Watcher, dir string, logger *slog.Logger) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}

	err = filepath.Walk(dir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr // skip unreadable entries
		}
		if fi.IsDir() {
			if addErr := watcher.Add(path); addErr != nil {
				logger.Warn("Failed to watch directory", "dir", path, "err", addErr)
			}
		}
		return nil
	})
	if err != nil {
		logger.Warn("Failed to walk refs directory", "dir", dir, "err", err)
	}
}

// statusPollLoop periodically recomputes working tree status and broadcasts
// if it has changed. This catches working-tree-only changes (new files, edits)
// that do not modify .git and would therefore be invisible to fsnotify.
func (s *Server) statusPollLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(statusPollInterval)
	defer ticker.Stop()

	var lastJSON []byte

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			repo := s.localSession.Repo()
			status := getWorkingTreeStatus(repo)
			if status == nil {
				continue
			}

			cur, err := json.Marshal(status)
			if err != nil {
				continue
			}

			if string(cur) == string(lastJSON) {
				continue
			}
			lastJSON = cur

			s.localSession.broadcastUpdate(UpdateMessage{Status: status})
		}
	}
}

func (s *Server) watchLoop(watcher *fsnotify.Watcher) {
	defer s.wg.Done()
	defer func() {
		if err := watcher.Close(); err != nil {
			s.logger.Error("Failed to close watcher", "err", err)
		}
	}()

	var debounceTimer *time.Timer

	for {
		select {
		case <-s.ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if shouldIgnoreEvent(event) {
				continue
			}

			s.logger.Debug("Change detected", "file", filepath.Base(event.Name), "op", event.Op.String())

			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceTime, func() {
				if s.ctx.Err() != nil {
					return
				}
				s.localSession.updateRepository()
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			s.logger.Error("Watcher error", "err", err)
		}
	}
}

func shouldIgnoreEvent(event fsnotify.Event) bool {
	base := filepath.Base(event.Name)
	path := event.Name

	// Accept Write, Create, Remove, and Rename events. Remove is critical
	// for detecting branch/tag deletion (the ref file is deleted from disk).
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return true
	}
	if strings.HasSuffix(base, ".lock") {
		return true
	}
	if strings.Contains(path, "/logs/") {
		return true
	}
	if base == "config" {
		return true
	}

	return false
}
