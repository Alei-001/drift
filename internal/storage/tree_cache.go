package storage

import (
	"container/list"
	"sync"

	"github.com/drift/drift/internal/core"
)

// maxTreeCacheEntries bounds the tree cache to prevent unbounded memory growth.
// 1024 trees covers deep directory structures while keeping memory predictable.
const maxTreeCacheEntries = 1024

// treeLRUCache is a bounded LRU cache for *core.Tree objects.
// Replaces the unbounded sync.Map (B4 fix): without eviction, long-running
// processes accumulated every tree ever read.
type treeLRUCache struct {
	mu    sync.Mutex
	items map[string]*list.Element
	ll    *list.List
}

type treeCacheEntry struct {
	hash string
	tree *core.Tree
}

func newTreeLRUCache() *treeLRUCache {
	return &treeLRUCache{
		items: make(map[string]*list.Element, maxTreeCacheEntries),
		ll:    list.New(),
	}
}

// Load returns the cached tree for hash, marking it as most-recently-used.
// ok is false if the hash is not in the cache.
func (c *treeLRUCache) Load(hash string) (*core.Tree, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[hash]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*treeCacheEntry).tree, true
	}
	return nil, false
}

// Store adds a tree to the cache, evicting the least-recently-used entry
// when the cache is full.
func (c *treeLRUCache) Store(hash string, t *core.Tree) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[hash]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*treeCacheEntry).tree = t
		return
	}
	el := c.ll.PushFront(&treeCacheEntry{hash: hash, tree: t})
	c.items[hash] = el
	if c.ll.Len() > maxTreeCacheEntries {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.items, oldest.Value.(*treeCacheEntry).hash)
		}
	}
}
