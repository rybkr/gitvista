package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/repositoryview"
)

func TestNewRepoSession(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
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
	repo := gitcore.NewEmptyRepository()
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
	repo := gitcore.NewEmptyRepository()
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
	repo := gitcore.NewEmptyRepository()
	rs := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
		// CacheSize: 0 — should default to defaultCacheSize
	})

	if rs.diffCache == nil {
		t.Error("diffCache was not initialized with default size")
	}
}

func TestRepoSession_DefaultLogger(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
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

func TestRepoSession_UpdateRepositoryClearsCaches(t *testing.T) {
	repo := gitcore.NewEmptyRepository()
	rs := NewRepoSession(SessionConfig{
		ID:          "test",
		InitialRepo: repo,
		ReloadFn:    func() (*gitcore.Repository, error) { return repo, nil },
		Logger:      silentLogger(),
	})

	rs.diffCache.Put("diff", "value")

	rs.updateRepository()

	if _, ok := rs.diffCache.Get("diff"); ok {
		t.Fatal("expected stale diff cache entry to be cleared")
	}
}

func TestMarshalPacketPayload(t *testing.T) {
	msg := UpdateMessage{
		Delta: &repositoryview.RepositoryDelta{
			AddedCommits: []*gitcore.Commit{
				{ID: gitcore.Hash("1111111111111111111111111111111111111111")},
				{ID: gitcore.Hash("2222222222222222222222222222222222222222")},
			},
		},
	}

	payload, commitCount, err := marshalPacketPayload(msg)
	if err != nil {
		t.Fatalf("marshalPacketPayload() error = %v", err)
	}
	if commitCount != 2 {
		t.Fatalf("commitCount = %d, want 2", commitCount)
	}

	expected, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(payload) != string(expected) {
		t.Fatalf("payload = %s, want %s", payload, expected)
	}
}

func TestLogPacketSent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	logPacketSent(logger, "bootstrap", 2, 42, 1024)

	out := buf.String()
	for _, want := range []string{
		"Packet sent",
		"type=bootstrap",
		"clients=2",
		"commits=42",
		"bytes=1024",
		"totalBytes=2048",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q: %s", want, out)
		}
	}
}
