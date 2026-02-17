package server

import (
	"github.com/fsnotify/fsnotify"
	"log"
	"path/filepath"
	"strings"
	"time"
)

const (
	debounceTime = 100 * time.Millisecond
)

// startWatcher initializes filesystem monitoring for the Git repository.
// It watches the .git/ directory for changes and triggers updates.
func (s *Server) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := watcher.Add(s.repo.GitDir()); err != nil {
		return err
	}

	s.wg.Add(1)
	go s.watchLoop(watcher)

	log.Printf("%s Watching Git repository for changes", logInfo)
	return nil
}

// watchLoop checks the .git/ folder for updates and updates the repository when one occurs.
// It uses a debounce timer to avoid flooding the system with updates.
func (s *Server) watchLoop(watcher *fsnotify.Watcher) {
	defer s.wg.Done()
	defer func() {
		if err := watcher.Close(); err != nil {
			log.Printf("failed to close watcher: %v", err)
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

			log.Printf("%s Change detected: %s", logInfo, filepath.Base(event.Name))

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
			log.Printf("%s Watcher error: %v", logError, err)
		}
	}
}

// shouldIgnoreEvent reports whether a file system event should be ignored for our purposes.
// For example, a change to an internal log file does not warrant a repository update.
func shouldIgnoreEvent(event fsnotify.Event) bool {
	base := filepath.Base(event.Name)
	path := event.Name

	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
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
