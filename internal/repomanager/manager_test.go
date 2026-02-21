package repomanager

import (
	"os"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		DataDir:             t.TempDir(),
		MaxConcurrentClones: 2,
		FetchInterval:       1 * time.Hour, // don't auto-fetch in tests
		InactivityTTL:       1 * time.Hour,
		CloneTimeout:        10 * time.Second,
		FetchTimeout:        10 * time.Second,
		MaxRepos:            10,
	}
}

func TestNew_CreatesDataDir(t *testing.T) {
	dir := t.TempDir() + "/nested/data"
	cfg := Config{DataDir: dir}

	rm, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer rm.Close()

	// Verify directory was created
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("data dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("data dir is not a directory")
	}
}

func TestAddRepo_Deduplication(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	id1, err := rm.AddRepo("https://github.com/user/repo.git")
	if err != nil {
		t.Fatalf("first AddRepo() error: %v", err)
	}

	id2, err := rm.AddRepo("https://github.com/user/repo.git")
	if err != nil {
		t.Fatalf("second AddRepo() error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("same URL got different IDs: %q vs %q", id1, id2)
	}

	repos := rm.List()
	if len(repos) != 1 {
		t.Errorf("List() returned %d repos, want 1", len(repos))
	}
}

func TestAddRepo_NormalizedDedup(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	id1, err := rm.AddRepo("https://github.com/user/repo.git")
	if err != nil {
		t.Fatalf("AddRepo(.git) error: %v", err)
	}

	id2, err := rm.AddRepo("https://github.com/user/repo")
	if err != nil {
		t.Fatalf("AddRepo(no .git) error: %v", err)
	}

	if id1 != id2 {
		t.Errorf("normalized URLs got different IDs: %q vs %q", id1, id2)
	}
}

func TestAddRepo_MaxRepos(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxRepos = 2
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	if _, err := rm.AddRepo("https://github.com/user/repo1"); err != nil {
		t.Fatalf("AddRepo 1 error: %v", err)
	}
	if _, err := rm.AddRepo("https://github.com/user/repo2"); err != nil {
		t.Fatalf("AddRepo 2 error: %v", err)
	}

	_, err = rm.AddRepo("https://github.com/user/repo3")
	if err == nil {
		t.Error("AddRepo should fail when MaxRepos reached")
	}
}

func TestAddRepo_InvalidURL(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	_, err = rm.AddRepo("")
	if err == nil {
		t.Error("AddRepo('') should fail")
	}

	_, err = rm.AddRepo("file:///local/repo")
	if err == nil {
		t.Error("AddRepo('file://') should fail")
	}
}

func TestGetRepo_NotFound(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	_, err = rm.GetRepo("nonexistent")
	if err == nil {
		t.Error("GetRepo() should fail for unknown ID")
	}
}

func TestGetRepo_Pending(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()
	// Don't start workers — repos stay pending

	id, err := rm.AddRepo("https://github.com/user/repo")
	if err != nil {
		t.Fatal(err)
	}

	_, err = rm.GetRepo(id)
	if err == nil {
		t.Error("GetRepo() should fail for pending repo")
	}
}

func TestStatus(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	id, err := rm.AddRepo("https://github.com/user/repo")
	if err != nil {
		t.Fatal(err)
	}

	state, _, err := rm.Status(id)
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if state != StatePending {
		t.Errorf("state = %v, want %v", state, StatePending)
	}

	_, _, err = rm.Status("nonexistent")
	if err == nil {
		t.Error("Status() should fail for unknown ID")
	}
}

func TestRemove(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	id, err := rm.AddRepo("https://github.com/user/repo")
	if err != nil {
		t.Fatal(err)
	}

	if err := rm.Remove(id); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	_, err = rm.GetRepo(id)
	if err == nil {
		t.Error("GetRepo() should fail after Remove()")
	}

	if err := rm.Remove(id); err == nil {
		t.Error("Remove() should fail for already-removed repo")
	}
}

func TestList(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	urls := []string{
		"https://github.com/user/repo1",
		"https://github.com/user/repo2",
		"https://github.com/user/repo3",
	}

	for _, u := range urls {
		if _, err := rm.AddRepo(u); err != nil {
			t.Fatalf("AddRepo(%q) error: %v", u, err)
		}
	}

	repos := rm.List()
	if len(repos) != 3 {
		t.Errorf("List() returned %d repos, want 3", len(repos))
	}
}

func TestClose_Graceful(t *testing.T) {
	cfg := testConfig(t)
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if err := rm.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Close should not panic or hang
	rm.Close()
}
