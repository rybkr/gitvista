package server

import (
	"encoding/json"
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
	if err := watcher.Add(repo.GitDir()); err != nil {
		return err
	}

	s.wg.Add(1)
	go s.statusPollLoop()

	go s.watchLoop(watcher)

	s.logger.Info("Watching Git repository for changes", "gitDir", repo.GitDir())
	return nil
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

			s.logger.Debug("Change detected", "file", filepath.Base(event.Name))

			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceTime, func() {
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

	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
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
