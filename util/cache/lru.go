package cache

import lru "github.com/hashicorp/golang-lru/v2"

type Cache[K comparable, V any] struct {
	lru *lru.Cache[K, V]
}

func NewCache[K comparable, V any](size int) (*Cache[K, V], error) {
	c, err := lru.New[K, V](size)
	if err != nil {
		return nil, err
	}
	return &Cache[K, V]{lru: c}, nil
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	return c.lru.Get(key)
}

func (c *Cache[K, V]) Add(key K, value V) bool {
	return c.lru.Add(key, value)
}

func (c *Cache[K, V]) Remove(key K) bool {
	return c.lru.Remove(key)
}

func (c *Cache[K, V]) Len() int {
	return c.lru.Len()
}
