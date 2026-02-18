package server

import (
	"container/list"
	"sync"
)

// LRUCache is a thread-safe, generic LRU cache backed by a doubly-linked list
// and a map for O(1) lookup. Front of the list = most recently used.
type LRUCache[V any] struct {
	mu      sync.Mutex
	maxSize int
	items   map[string]*list.Element
	order   *list.List
}

// entry wraps a cached value with its key for LRU eviction.
type entry[V any] struct {
	key   string
	value V
}

// NewLRUCache creates a new LRU cache with the given max size.
// If maxSize <= 0, defaults to 500.
func NewLRUCache[V any](maxSize int) *LRUCache[V] {
	if maxSize <= 0 {
		maxSize = 500
	}
	return &LRUCache[V]{
		maxSize: maxSize,
		items:   make(map[string]*list.Element),
		order:   list.New(),
	}
}

// Get retrieves a value from the cache and moves it to the front (MRU).
// Returns (value, true) on hit; (zero, false) on miss.
func (c *LRUCache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	// Promote to front (MRU).
	c.order.MoveToFront(elem)
	return elem.Value.(entry[V]).value, true
}

// Put inserts or updates a key-value pair in the cache.
// If the cache is full, evicts the least recently used entry.
func (c *LRUCache[V]) Put(key string, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If key exists, update in-place and move to front.
	if elem, ok := c.items[key]; ok {
		elem.Value = entry[V]{key, val}
		c.order.MoveToFront(elem)
		return
	}

	// Add new entry at front.
	elem := c.order.PushFront(entry[V]{key, val})
	c.items[key] = elem

	// Evict LRU if over capacity.
	if c.order.Len() > c.maxSize {
		lru := c.order.Back()
		c.order.Remove(lru)
		delete(c.items, lru.Value.(entry[V]).key)
	}
}

// Clear empties the cache.
func (c *LRUCache[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.order = list.New()
}

// Len returns the current number of entries in the cache.
func (c *LRUCache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}
