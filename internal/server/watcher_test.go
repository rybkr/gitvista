package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestWatchNewDirectory_AddsCreatedDirectorySubtree(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(noopWriter{}, nil))

	root := t.TempDir()
	refsHeads := filepath.Join(root, "refs", "heads")
	if err := os.MkdirAll(refsHeads, 0o755); err != nil {
		t.Fatalf("MkdirAll refs/heads: %v", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	defer watcher.Close()

	walkAndWatch(watcher, refsHeads, logger)

	featureDir := filepath.Join(refsHeads, "feature")
	nestedDir := filepath.Join(featureDir, "login")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll nested ref dir: %v", err)
	}

	watchNewDirectory(watcher, fsnotify.Event{
		Name: featureDir,
		Op:   fsnotify.Create,
	}, logger)

	watched := watcher.WatchList()
	if !containsPath(watched, featureDir) {
		t.Fatalf("watch list missing created directory %q: %v", featureDir, watched)
	}
	if !containsPath(watched, nestedDir) {
		t.Fatalf("watch list missing nested directory %q: %v", nestedDir, watched)
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
