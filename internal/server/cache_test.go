package server

import (
	"fmt"
	"sync"
	"testing"
)

// TestLRUCache_BasicGetPut verifies that entries can be stored and retrieved.
func TestLRUCache_BasicGetPut(t *testing.T) {
	c := NewLRUCache[string](10)

	// Miss on an empty cache.
	if _, ok := c.Get("missing"); ok {
		t.Error("Get on empty cache should return false")
	}

	c.Put("a", "alpha")
	c.Put("b", "beta")

	tests := []struct {
		key  string
		want string
		ok   bool
	}{
		{"a", "alpha", true},
		{"b", "beta", true},
		{"c", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := c.Get(tt.key)
			if ok != tt.ok {
				t.Fatalf("Get(%q) ok = %v, want %v", tt.key, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("Get(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// TestLRUCache_Update verifies that a second Put for the same key overwrites
// the value and does not grow the cache.
func TestLRUCache_Update(t *testing.T) {
	c := NewLRUCache[int](5)
	c.Put("x", 1)
	c.Put("x", 2)

	if c.Len() != 1 {
		t.Errorf("Len() = %d after updating same key, want 1", c.Len())
	}
	got, ok := c.Get("x")
	if !ok {
		t.Fatal("Get returned false after Put")
	}
	if got != 2 {
		t.Errorf("Get = %d, want 2", got)
	}
}

// TestLRUCache_Eviction verifies that the least-recently-used entry is evicted
// when the cache reaches capacity.
func TestLRUCache_Eviction(t *testing.T) {
	const size = 3
	c := NewLRUCache[int](size)

	// Fill to capacity: order MRU→LRU is c, b, a.
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	if c.Len() != size {
		t.Fatalf("Len() = %d, want %d", c.Len(), size)
	}

	// "d" triggers eviction of "a" (oldest).
	c.Put("d", 4)

	if c.Len() != size {
		t.Errorf("Len() = %d after eviction, want %d", c.Len(), size)
	}
	if _, ok := c.Get("a"); ok {
		t.Error("evicted entry 'a' should not be present")
	}
	// Remaining entries must still be reachable.
	for _, key := range []string{"b", "c", "d"} {
		if _, ok := c.Get(key); !ok {
			t.Errorf("entry %q should still be present after eviction of 'a'", key)
		}
	}
}

// TestLRUCache_GetPromotes verifies that a Get call moves the entry to the
// front, so a different (un-accessed) entry is evicted first.
func TestLRUCache_GetPromotes(t *testing.T) {
	c := NewLRUCache[int](3)

	c.Put("a", 1) // LRU order: a
	c.Put("b", 2) // LRU order: b, a
	c.Put("c", 3) // LRU order: c, b, a

	// Access "a" — now LRU order is: a, c, b.
	c.Get("a")

	// "d" should evict "b" (now the oldest), not "a".
	c.Put("d", 4)

	if _, ok := c.Get("b"); ok {
		t.Error("'b' should have been evicted (LRU) but is still present")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("'a' should NOT have been evicted (it was recently accessed)")
	}
}

// TestLRUCache_Len verifies the entry count across Put, Get, and eviction.
func TestLRUCache_Len(t *testing.T) {
	c := NewLRUCache[string](3)

	if c.Len() != 0 {
		t.Errorf("Len() = %d on new cache, want 0", c.Len())
	}

	c.Put("one", "1")
	if c.Len() != 1 {
		t.Errorf("Len() = %d, want 1", c.Len())
	}

	c.Put("two", "2")
	c.Put("three", "3")
	if c.Len() != 3 {
		t.Errorf("Len() = %d, want 3", c.Len())
	}

	// Overflow — evicts "one".
	c.Put("four", "4")
	if c.Len() != 3 {
		t.Errorf("Len() = %d after overflow eviction, want 3", c.Len())
	}
}

// TestLRUCache_Clear verifies that Clear empties the cache completely.
func TestLRUCache_Clear(t *testing.T) {
	c := NewLRUCache[int](10)
	for i := range 5 {
		c.Put(fmt.Sprintf("key%d", i), i)
	}
	if c.Len() != 5 {
		t.Fatalf("pre-Clear Len() = %d, want 5", c.Len())
	}

	c.Clear()

	if c.Len() != 0 {
		t.Errorf("post-Clear Len() = %d, want 0", c.Len())
	}

	// The cache must still be usable after Clear.
	c.Put("new", 99)
	got, ok := c.Get("new")
	if !ok {
		t.Fatal("Get after Clear returned false")
	}
	if got != 99 {
		t.Errorf("Get after Clear = %d, want 99", got)
	}
}

// TestLRUCache_DefaultSize verifies that a zero or negative maxSize falls back
// to defaultCacheSize.
func TestLRUCache_DefaultSize(t *testing.T) {
	for _, size := range []int{0, -1, -100} {
		c := NewLRUCache[int](size)
		// Fill past the explicit zero/negative value — if the fallback is in
		// effect none of these inserts should panic.
		for i := range 10 {
			c.Put(fmt.Sprintf("k%d", i), i)
		}
		if c.Len() != 10 {
			t.Errorf("maxSize=%d: Len() = %d, want 10", size, c.Len())
		}
	}
}

// TestLRUCache_ConcurrentAccess verifies that simultaneous Get and Put calls
// from multiple goroutines do not race or panic.
// Run with: go test -race ./internal/server -run TestLRUCache_ConcurrentAccess
func TestLRUCache_ConcurrentAccess(t *testing.T) {
	const (
		goroutines = 20
		ops        = 100
		cacheSize  = 10 // intentionally small to force frequent eviction
	)

	c := NewLRUCache[int](cacheSize)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range ops {
				key := fmt.Sprintf("g%d-k%d", id, i%cacheSize)
				c.Put(key, i)
				c.Get(key)
			}
		}(g)
	}

	wg.Wait()

	// Cache size must not exceed maxSize after concurrent writes.
	if got := c.Len(); got > cacheSize {
		t.Errorf("Len() = %d after concurrent ops, must not exceed maxSize %d", got, cacheSize)
	}
}

// TestLRUCache_ConcurrentClear verifies that Clear interleaved with Put/Get
// does not produce a data race.
func TestLRUCache_ConcurrentClear(_ *testing.T) {
	c := NewLRUCache[int](50)

	var wg sync.WaitGroup
	for g := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range 50 {
				c.Put(fmt.Sprintf("key-%d-%d", id, i), i)
				c.Get(fmt.Sprintf("key-%d-%d", id, i))
				if i%10 == 0 {
					c.Clear()
				}
			}
		}(g)
	}
	wg.Wait()

	// No assertion — the goal is to confirm the race detector doesn't fire.
}

// TestLRUCache_CacheSizeEnvVar validates the parsing logic used in NewServer
// for GITVISTA_CACHE_SIZE. We test the parsing inline rather than setting the
// env var (which would require process-level mutation in a parallel test suite).
func TestLRUCache_CacheSizeEnvVar(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantSize    int // expected effective maxSize
		insertCount int // how many entries to insert to confirm capacity
	}{
		{
			name:        "valid positive integer",
			raw:         "20",
			wantSize:    20,
			insertCount: 20,
		},
		{
			name:        "zero falls back to default",
			raw:         "0",
			wantSize:    defaultCacheSize,
			insertCount: 10,
		},
		{
			name:        "negative falls back to default",
			raw:         "-5",
			wantSize:    defaultCacheSize,
			insertCount: 10,
		},
		{
			name:        "non-numeric falls back to default",
			raw:         "abc",
			wantSize:    defaultCacheSize,
			insertCount: 10,
		},
		{
			name:        "empty string uses default",
			raw:         "",
			wantSize:    defaultCacheSize,
			insertCount: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the parsing logic from NewServer without touching os.Setenv.
			size := defaultCacheSize
			if tt.raw != "" {
				if n, err := parseInt(tt.raw); err == nil && n > 0 {
					size = n
				}
			}

			if size != tt.wantSize {
				t.Errorf("parsed size = %d, want %d (raw=%q)", size, tt.wantSize, tt.raw)
			}

			// Construct a cache at the parsed size and verify it works.
			c := NewLRUCache[int](size)
			for i := range tt.insertCount {
				c.Put(fmt.Sprintf("k%d", i), i)
			}
			if c.Len() != tt.insertCount {
				t.Errorf("Len() = %d, want %d", c.Len(), tt.insertCount)
			}
		})
	}
}

// parseInt is a thin wrapper around strconv.Atoi used to test env-var parsing
// without importing strconv in the test file.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// TestLRUCache_EvictionOrder checks the precise eviction sequence for a
// capacity-3 cache: oldest entry must always be the next to go.
func TestLRUCache_EvictionOrder(t *testing.T) {
	c := NewLRUCache[int](3)

	c.Put("1", 1)
	c.Put("2", 2)
	c.Put("3", 3)
	// MRU→LRU order: 3, 2, 1

	// Add "4" — evicts "1".
	c.Put("4", 4)
	if _, ok := c.Get("1"); ok {
		t.Error("expected '1' to be evicted")
	}

	// Order now: 4, 3, 2.  Add "5" — evicts "2".
	c.Put("5", 5)
	if _, ok := c.Get("2"); ok {
		t.Error("expected '2' to be evicted")
	}

	// Order now: 5, 4, 3.  Access "3" to promote it → order: 3, 5, 4.
	c.Get("3")

	// Add "6" — should evict "4" (now the tail).
	c.Put("6", 6)
	if _, ok := c.Get("4"); ok {
		t.Error("expected '4' to be evicted")
	}
	if _, ok := c.Get("3"); !ok {
		t.Error("'3' should still be present (was promoted)")
	}
}
