package server

import (
	"container/list"
	"sync"
)

// defaultCacheSize is the number of entries held before the LRU evicts the
// oldest entry. 500 covers typical usage patterns (hundreds of unique
// commit+path combinations) without unbounded memory growth.
const defaultCacheSize = 500

// lruEntry is the value stored inside each list.Element so we can delete the
// map key without a separate lookup when evicting.
type lruEntry[V any] struct {
	key string
	val V
}

// LRUCache is a generic, thread-safe least-recently-used cache bounded by an
// entry count. Every Get is a write operation (it moves the entry to the front
// of the list), so a plain sync.Mutex is used rather than sync.RWMutex.
//
// The zero value is not usable; always construct via NewLRUCache.
type LRUCache[V any] struct {
	mu      sync.Mutex
	maxSize int
	items   map[string]*list.Element // O(1) key lookup
	order   *list.List               // front = most-recently-used
}

// NewLRUCache constructs an LRUCache that holds at most maxSize entries.
// If maxSize is <= 0 it is set to defaultCacheSize.
func NewLRUCache[V any](maxSize int) *LRUCache[V] {
	if maxSize <= 0 {
		maxSize = defaultCacheSize
	}
	return &LRUCache[V]{
		maxSize: maxSize,
		items:   make(map[string]*list.Element, maxSize),
		order:   list.New(),
	}
}

// Get returns the value associated with key and true if found.
// A cache hit moves the entry to the most-recently-used position.
func (c *LRUCache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	// Promote to front — this is why every Get is a mutex write.
	c.order.MoveToFront(elem)
	return elem.Value.(*lruEntry[V]).val, true
}

// Put inserts or updates key with val and moves it to most-recently-used.
// When the cache is at capacity the least-recently-used entry is evicted first.
func (c *LRUCache[V]) Put(key string, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry in-place to avoid an unnecessary eviction.
	if elem, ok := c.items[key]; ok {
		elem.Value.(*lruEntry[V]).val = val
		c.order.MoveToFront(elem)
		return
	}

	// Evict the LRU entry before inserting to keep Len <= maxSize.
	if len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	entry := &lruEntry[V]{key: key, val: val}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

// evictOldest removes the least-recently-used entry. Must be called with mu held.
func (c *LRUCache[V]) evictOldest() {
	oldest := c.order.Back()
	if oldest == nil {
		return
	}
	c.order.Remove(oldest)
	delete(c.items, oldest.Value.(*lruEntry[V]).key)
}

// Clear removes all entries from the cache.
func (c *LRUCache[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element, c.maxSize)
	c.order.Init()
}

// Len returns the current number of entries in the cache.
func (c *LRUCache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
