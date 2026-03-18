package hosted

import (
	"strings"
	"testing"

	"github.com/rybkr/gitvista/internal/repomanager"
)

func newTestRepoManager(t *testing.T) *repomanager.RepoManager {
	t.Helper()
	rm, err := repomanager.New(repomanager.Config{
		DataDir: t.TempDir(),
		Logger:  silentLogger(),
	})
	if err != nil {
		t.Fatalf("failed to create repo manager: %v", err)
	}
	t.Cleanup(rm.Close)
	return rm
}

func TestNewHostedStore_DefaultsToMemory(t *testing.T) {
	rm := newTestRepoManager(t)

	store, err := NewHostedStore(rm, "")
	if err != nil {
		t.Fatalf("NewHostedStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, ok := store.(*memoryHostedStore); !ok {
		t.Fatalf("store type = %T, want *memoryHostedStore", store)
	}
}

func TestNewHostedStore_PostgresDriverMissing(t *testing.T) {
	rm := newTestRepoManager(t)

	_, err := NewHostedStore(rm, "postgres://gitvista:secret@localhost:5432/gitvista?sslmode=disable")
	if err == nil {
		t.Fatal("expected error when postgres driver is unavailable")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("error = %q, want mention of postgres", err)
	}
}
