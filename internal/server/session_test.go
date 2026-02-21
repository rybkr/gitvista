package server

import (
	"testing"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

func TestNewRepoSession(t *testing.T) {
	repo := &gitcore.Repository{}
	rs := NewRepoSession(SessionConfig{
		ID:          "test-session",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		CacheSize:   100,
		Logger:      silentLogger(),
	})

	if rs.id != "test-session" {
		t.Errorf("id = %q, want %q", rs.id, "test-session")
	}
	if rs.logger == nil {
		t.Error("logger is nil")
	}
	if rs.reloadFn == nil {
		t.Error("reloadFn is nil")
	}
	if rs.clients == nil {
		t.Error("clients map is nil")
	}
	if rs.broadcast == nil {
		t.Error("broadcast channel is nil")
	}
	if rs.blameCache == nil {
		t.Error("blameCache is nil")
	}
	if rs.diffCache == nil {
		t.Error("diffCache is nil")
	}
	if rs.ctx == nil {
		t.Error("ctx is nil")
	}
	if rs.cancel == nil {
		t.Error("cancel is nil")
	}
}

func TestRepoSession_Repo(t *testing.T) {
	repo := &gitcore.Repository{}
	rs := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
	})

	got := rs.Repo()
	if got != repo {
		t.Error("Repo() did not return the initial repository")
	}
}

func TestRepoSession_Close(t *testing.T) {
	repo := &gitcore.Repository{}
	rs := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
	})

	rs.Start()

	done := make(chan struct{})
	go func() {
		rs.Close()
		close(done)
	}()

	select {
	case <-done:
		// Verify context was canceled
		select {
		case <-rs.ctx.Done():
		default:
			t.Error("context was not canceled after Close()")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not complete within 5 seconds")
	}
}

func TestRepoSession_DefaultCacheSize(t *testing.T) {
	repo := &gitcore.Repository{}
	rs := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
		// CacheSize: 0 — should default to defaultCacheSize
	})

	if rs.blameCache == nil {
		t.Error("blameCache was not initialized with default size")
	}
}

func TestRepoSession_DefaultLogger(t *testing.T) {
	repo := &gitcore.Repository{}
	rs := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		// Logger: nil — should default to slog.Default()
	})

	if rs.logger == nil {
		t.Error("logger was not initialized with default")
	}
}
