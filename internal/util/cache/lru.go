package cache

import lru "github.com/hashicorp/golang-lru/v2"

// Cache is a generic fixed-size LRU cache wrapping hashicorp/golang-lru/v2.
type Cache[K comparable, V any] struct {
	lru *lru.Cache[K, V]
}

// NewCache creates an LRU Cache with the given maximum size. A non-positive
// size returns an error from the underlying implementation.
func NewCache[K comparable, V any](size int) (*Cache[K, V], error) {
	c, err := lru.New[K, V](size)
	if err != nil {
		return nil, err
	}
	return &Cache[K, V]{lru: c}, nil
}

// Get returns the value for key and reports whether it was present. Accessing
// a key marks it as most-recently-used.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	return c.lru.Get(key)
}

// Add inserts or updates key with value, evicting the least-recently-used
// entry if the cache is full. It returns true if an eviction occurred.
func (c *Cache[K, V]) Add(key K, value V) bool {
	return c.lru.Add(key, value)
}

// Remove removes key from the cache. It returns true if key was present.
func (c *Cache[K, V]) Remove(key K) bool {
	return c.lru.Remove(key)
}

// Len returns the number of entries currently in the cache.
func (c *Cache[K, V]) Len() int {
	return c.lru.Len()
}
