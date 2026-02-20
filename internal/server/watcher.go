package server

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounceTime = 100 * time.Millisecond

func (s *Server) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := watcher.Add(s.repo.GitDir()); err != nil {
		return err
	}

	go s.watchLoop(watcher)

	s.logger.Info("Watching Git repository for changes", "gitDir", s.repo.GitDir())
	return nil
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

			// Log at Debug to avoid flooding logs on active repos; operators
			// who need per-file visibility can enable GITVISTA_LOG_LEVEL=debug.
			s.logger.Debug("Change detected", "file", filepath.Base(event.Name))

			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(debounceTime, func() {
				s.updateRepository()
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

	// Git uses atomic renames for many operations (pack index updates, ref updates).
	// Rename events must not be ignored or branch/commit changes will be missed.
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
