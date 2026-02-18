package server

import (
	"fmt"
	"sync"
	"testing"
)

// ── NewLRUCache ─────────────────────────────────────────────────────────────

func TestNewLRUCache_DefaultsToFiveHundredWhenZero(t *testing.T) {
	c := NewLRUCache[string](0)
	// Fill to 501 to verify the effective cap is 500, not 0.
	for i := range 501 {
		c.Put(fmt.Sprintf("k%d", i), "v")
	}
	if c.Len() != 500 {
		t.Errorf("Len() = %d, want 500 (default cap enforced)", c.Len())
	}
}

func TestNewLRUCache_DefaultsToFiveHundredWhenNegative(t *testing.T) {
	c := NewLRUCache[int](-100)
	for i := range 501 {
		c.Put(fmt.Sprintf("k%d", i), i)
	}
	if c.Len() != 500 {
		t.Errorf("Len() = %d, want 500 (default cap enforced for negative size)", c.Len())
	}
}

func TestNewLRUCache_RespectsPositiveMaxSize(t *testing.T) {
	c := NewLRUCache[string](3)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	if c.Len() != 3 {
		t.Errorf("Len() = %d, want 3", c.Len())
	}
}

// ── Get ─────────────────────────────────────────────────────────────────────

func TestGet_MissOnEmptyCache(t *testing.T) {
	c := NewLRUCache[string](10)
	_, ok := c.Get("missing")
	if ok {
		t.Error("Get on empty cache returned ok=true, want false")
	}
}

func TestGet_MissOnAbsentKey(t *testing.T) {
	c := NewLRUCache[string](10)
	c.Put("existing", "value")
	_, ok := c.Get("absent")
	if ok {
		t.Error("Get for absent key returned ok=true, want false")
	}
}

func TestGet_HitReturnsCorrectValue(t *testing.T) {
	c := NewLRUCache[string](10)
	c.Put("key", "hello")
	v, ok := c.Get("key")
	if !ok {
		t.Fatal("Get returned ok=false, want true")
	}
	if v != "hello" {
		t.Errorf("Get() = %q, want %q", v, "hello")
	}
}

func TestGet_ZeroValueReturnedOnMiss(t *testing.T) {
	c := NewLRUCache[int](10)
	v, ok := c.Get("nope")
	if ok {
		t.Error("Get on absent key returned ok=true")
	}
	if v != 0 {
		t.Errorf("Get() zero value = %d, want 0", v)
	}
}

// ── Put ─────────────────────────────────────────────────────────────────────

func TestPut_OverwritesExistingKey(t *testing.T) {
	c := NewLRUCache[string](10)
	c.Put("k", "first")
	c.Put("k", "second")
	v, ok := c.Get("k")
	if !ok {
		t.Fatal("Get after overwrite returned ok=false")
	}
	if v != "second" {
		t.Errorf("overwritten value = %q, want %q", v, "second")
	}
	if c.Len() != 1 {
		t.Errorf("Len() after overwrite = %d, want 1 (no duplicate entries)", c.Len())
	}
}

func TestPut_IncreasesLen(t *testing.T) {
	c := NewLRUCache[string](10)
	for i := range 5 {
		c.Put(fmt.Sprintf("k%d", i), "v")
		if c.Len() != i+1 {
			t.Errorf("after %d puts Len()=%d, want %d", i+1, c.Len(), i+1)
		}
	}
}

// ── LRU Eviction ─────────────────────────────────────────────────────────────

func TestEviction_LRUEntryRemovedWhenFull(t *testing.T) {
	c := NewLRUCache[string](3)
	// Insert in order: a, b, c → a is the LRU.
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")

	// Adding "d" must evict "a" (oldest, LRU).
	c.Put("d", "4")

	if c.Len() != 3 {
		t.Errorf("Len() = %d, want 3", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Error("key 'a' (LRU) should have been evicted but is still present")
	}
	for _, key := range []string{"b", "c", "d"} {
		if _, ok := c.Get(key); !ok {
			t.Errorf("key %q should still be present after eviction", key)
		}
	}
}

func TestEviction_TableDrivenEvictionOrder(t *testing.T) {
	// Each test verifies which key survives a sequence of puts and gets.
	tests := []struct {
		name       string
		maxSize    int
		ops        []func(c *LRUCache[int])
		expectHit  []string
		expectMiss []string
	}{
		{
			name:    "insert_three_then_one_more_evicts_first",
			maxSize: 3,
			ops: []func(c *LRUCache[int]){
				func(c *LRUCache[int]) { c.Put("a", 1) },
				func(c *LRUCache[int]) { c.Put("b", 2) },
				func(c *LRUCache[int]) { c.Put("c", 3) },
				func(c *LRUCache[int]) { c.Put("d", 4) },
			},
			expectHit:  []string{"b", "c", "d"},
			expectMiss: []string{"a"},
		},
		{
			name:    "get_promotes_so_different_key_evicted",
			maxSize: 3,
			ops: []func(c *LRUCache[int]){
				func(c *LRUCache[int]) { c.Put("a", 1) },
				func(c *LRUCache[int]) { c.Put("b", 2) },
				func(c *LRUCache[int]) { c.Put("c", 3) },
				// Access "a" to make it MRU; "b" becomes LRU.
				func(c *LRUCache[int]) { c.Get("a") },
				func(c *LRUCache[int]) { c.Put("d", 4) },
			},
			expectHit:  []string{"a", "c", "d"},
			expectMiss: []string{"b"},
		},
		{
			name:    "overwrite_promotes_to_front_so_different_key_evicted",
			maxSize: 3,
			ops: []func(c *LRUCache[int]){
				func(c *LRUCache[int]) { c.Put("a", 1) },
				func(c *LRUCache[int]) { c.Put("b", 2) },
				func(c *LRUCache[int]) { c.Put("c", 3) },
				// Re-put "a" to make it MRU; "b" is now LRU.
				func(c *LRUCache[int]) { c.Put("a", 99) },
				func(c *LRUCache[int]) { c.Put("d", 4) },
			},
			expectHit:  []string{"a", "c", "d"},
			expectMiss: []string{"b"},
		},
		{
			name:    "capacity_one_always_evicts_previous",
			maxSize: 1,
			ops: []func(c *LRUCache[int]){
				func(c *LRUCache[int]) { c.Put("a", 1) },
				func(c *LRUCache[int]) { c.Put("b", 2) },
			},
			expectHit:  []string{"b"},
			expectMiss: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewLRUCache[int](tt.maxSize)
			for _, op := range tt.ops {
				op(c)
			}
			for _, key := range tt.expectHit {
				if _, ok := c.Get(key); !ok {
					t.Errorf("key %q expected to be present but is missing", key)
				}
			}
			for _, key := range tt.expectMiss {
				if _, ok := c.Get(key); ok {
					t.Errorf("key %q expected to be evicted but is still present", key)
				}
			}
		})
	}
}

// ── Get promotes to MRU ──────────────────────────────────────────────────────

func TestGet_PromotesEntryToMRU(t *testing.T) {
	c := NewLRUCache[string](3)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")

	// "a" is currently LRU. Access it to promote it.
	c.Get("a")

	// Now "b" should be LRU. Adding "d" should evict "b".
	c.Put("d", "4")

	if _, ok := c.Get("b"); ok {
		t.Error("key 'b' should have been evicted (was LRU after 'a' was promoted)")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("key 'a' should still be present (was promoted to MRU by Get)")
	}
}

// ── Clear ────────────────────────────────────────────────────────────────────

func TestClear_EmptiesCache(t *testing.T) {
	c := NewLRUCache[string](10)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("c", "3")
	c.Clear()

	if c.Len() != 0 {
		t.Errorf("Len() after Clear() = %d, want 0", c.Len())
	}
}

func TestClear_KeysNoLongerReachable(t *testing.T) {
	c := NewLRUCache[string](10)
	c.Put("x", "val")
	c.Clear()
	if _, ok := c.Get("x"); ok {
		t.Error("Get after Clear() returned ok=true for previously cached key")
	}
}

func TestClear_CacheUsableAfterClear(t *testing.T) {
	c := NewLRUCache[string](3)
	c.Put("a", "1")
	c.Clear()
	c.Put("b", "2")
	c.Put("c", "3")
	c.Put("d", "4")

	if c.Len() != 3 {
		t.Errorf("Len() = %d, want 3 (cache should work normally after Clear)", c.Len())
	}
}

// ── Len ─────────────────────────────────────────────────────────────────────

func TestLen_ReflectsPutAndEviction(t *testing.T) {
	c := NewLRUCache[string](2)
	if c.Len() != 0 {
		t.Errorf("Len() on new cache = %d, want 0", c.Len())
	}
	c.Put("a", "1")
	if c.Len() != 1 {
		t.Errorf("Len() after 1 put = %d, want 1", c.Len())
	}
	c.Put("b", "2")
	if c.Len() != 2 {
		t.Errorf("Len() after 2 puts = %d, want 2", c.Len())
	}
	// Adding a 3rd entry evicts one — Len stays at 2.
	c.Put("c", "3")
	if c.Len() != 2 {
		t.Errorf("Len() after eviction = %d, want 2", c.Len())
	}
}

// ── Concurrency (race-detector target) ──────────────────────────────────────

func TestLRUCache_ConcurrentPutGet_NoDataRace(t *testing.T) {
	const goroutines = 8
	const opsPerGoroutine = 200
	c := NewLRUCache[int](50)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range opsPerGoroutine {
				key := fmt.Sprintf("g%d-k%d", id, i%20) // deliberate key overlap
				c.Put(key, i)
				c.Get(key)
			}
		}(g)
	}

	wg.Wait()
	// Len must be <= maxSize; exact value depends on scheduling.
	if c.Len() > 50 {
		t.Errorf("Len() = %d exceeds maxSize=50 after concurrent ops", c.Len())
	}
}

func TestLRUCache_ConcurrentPutClear_NoDataRace(t *testing.T) {
	c := NewLRUCache[string](100)
	var wg sync.WaitGroup

	// Writers
	for g := range 4 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range 100 {
				c.Put(fmt.Sprintf("w%d-k%d", id, i), "v")
			}
		}(g)
	}

	// Clearers
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				c.Clear()
			}
		}()
	}

	wg.Wait()
	// After all goroutines finish, Len() must be consistent.
	l := c.Len()
	if l < 0 || l > 100 {
		t.Errorf("Len() = %d is out of valid range [0, 100]", l)
	}
}

// ── parseCacheSize ────────────────────────────────────────────────────────────

func TestParseCacheSize_ReturnsDefaultWhenEnvUnset(t *testing.T) {
	// Use a bogus env var name that is guaranteed to be unset.
	got := parseCacheSize("GITVISTA_TEST_CACHE_SIZE_UNSET_XYZ", 42)
	if got != 42 {
		t.Errorf("parseCacheSize() = %d, want default 42", got)
	}
}

func TestParseCacheSize_TableDriven(t *testing.T) {
	const envVar = "GITVISTA_TEST_CACHE_SIZE"
	const defaultSize = 100

	tests := []struct {
		name    string
		envVal  string   // value to set; "" means unset
		want    int
	}{
		{
			name:   "unset_env_uses_default",
			envVal: "",
			want:   defaultSize,
		},
		{
			name:   "valid_positive_integer",
			envVal: "250",
			want:   250,
		},
		{
			name:   "zero_falls_back_to_default",
			envVal: "0",
			want:   defaultSize,
		},
		{
			name:   "negative_falls_back_to_default",
			envVal: "-1",
			want:   defaultSize,
		},
		{
			name:   "non_numeric_falls_back_to_default",
			envVal: "abc",
			want:   defaultSize,
		},
		{
			name:   "float_falls_back_to_default",
			envVal: "3.14",
			want:   defaultSize,
		},
		{
			name:   "whitespace_falls_back_to_default",
			envVal: "  50  ",
			want:   defaultSize,
		},
		{
			name:   "very_large_value_accepted",
			envVal: "99999",
			want:   99999,
		},
		{
			name:   "minimum_valid_value",
			envVal: "1",
			want:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envVal == "" {
				t.Setenv(envVar, "")
			} else {
				t.Setenv(envVar, tt.envVal)
			}
			got := parseCacheSize(envVar, defaultSize)
			if got != tt.want {
				t.Errorf("parseCacheSize(%q) = %d, want %d", tt.envVal, got, tt.want)
			}
		})
	}
}

// ── Generic type parameter tests ─────────────────────────────────────────────

func TestLRUCache_WorksWithAnyType(t *testing.T) {
	// Verify the generic constraint works for slices and structs in addition to
	// the string/int used elsewhere.
	type payload struct {
		name string
		val  int
	}

	c := NewLRUCache[payload](5)
	c.Put("k1", payload{"alice", 1})
	c.Put("k2", payload{"bob", 2})

	v, ok := c.Get("k1")
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	if v.name != "alice" || v.val != 1 {
		t.Errorf("value = %+v, want {alice 1}", v)
	}
}

func TestLRUCache_WorksWithSliceType(t *testing.T) {
	c := NewLRUCache[[]string](5)
	c.Put("rows", []string{"a", "b", "c"})
	v, ok := c.Get("rows")
	if !ok {
		t.Fatal("Get returned ok=false for slice type")
	}
	if len(v) != 3 || v[0] != "a" {
		t.Errorf("retrieved slice = %v, want [a b c]", v)
	}
}
