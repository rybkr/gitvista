package repomanager

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEvictInactive(t *testing.T) {
	cfg := testConfig(t)
	cfg.InactivityTTL = 1 * time.Millisecond
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	// Create a fake ready repo with old LastAccess
	id := "test-evict"
	diskPath := filepath.Join(cfg.DataDir, id)
	if err := os.MkdirAll(diskPath, 0o755); err != nil {
		t.Fatal(err)
	}

	rm.mu.Lock()
	rm.repos[id] = &ManagedRepo{
		ID:         id,
		State:      StateReady,
		DiskPath:   diskPath,
		LastAccess: time.Now().Add(-1 * time.Hour),
	}
	rm.mu.Unlock()

	// Run eviction directly
	rm.evictInactive()

	rm.mu.RLock()
	_, exists := rm.repos[id]
	rm.mu.RUnlock()

	if exists {
		t.Error("expected repo to be evicted")
	}

	if _, err := os.Stat(diskPath); err == nil {
		t.Error("expected disk path to be removed")
	}
}

func TestEvictInactive_SkipsPending(t *testing.T) {
	cfg := testConfig(t)
	cfg.InactivityTTL = 1 * time.Millisecond
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	id := "test-pending"
	rm.mu.Lock()
	rm.repos[id] = &ManagedRepo{
		ID:         id,
		State:      StatePending,
		DiskPath:   filepath.Join(cfg.DataDir, id),
		LastAccess: time.Now().Add(-1 * time.Hour),
	}
	rm.mu.Unlock()

	rm.evictInactive()

	rm.mu.RLock()
	_, exists := rm.repos[id]
	rm.mu.RUnlock()

	if !exists {
		t.Error("pending repo should not be evicted")
	}
}

func TestEvictInactive_KeepsActive(t *testing.T) {
	cfg := testConfig(t)
	cfg.InactivityTTL = 1 * time.Hour
	rm, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rm.Close()

	id := "test-active"
	rm.mu.Lock()
	rm.repos[id] = &ManagedRepo{
		ID:         id,
		State:      StateReady,
		DiskPath:   filepath.Join(cfg.DataDir, id),
		LastAccess: time.Now(),
	}
	rm.mu.Unlock()

	rm.evictInactive()

	rm.mu.RLock()
	_, exists := rm.repos[id]
	rm.mu.RUnlock()

	if !exists {
		t.Error("active repo should not be evicted")
	}
}
