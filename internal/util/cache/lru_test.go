package cache

import "testing"

func TestNewCache_InvalidSize(t *testing.T) {
	// hashicorp/golang-lru requires size >= 1; 0 returns an error.
	_, err := NewCache[int, int](0)
	if err == nil {
		t.Error("expected error for size 0, got nil")
	}
}

func TestNewCache_ValidSize(t *testing.T) {
	c, err := NewCache[int, int](10)
	if err != nil {
		t.Fatalf("NewCache(10) failed: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
}

func TestCache_GetMiss(t *testing.T) {
	c, _ := NewCache[string, int](10)
	_, ok := c.Get("missing")
	if ok {
		t.Error("expected miss for missing key, got hit")
	}
}

func TestCache_AddAndGet(t *testing.T) {
	c, _ := NewCache[string, int](10)
	c.Add("key", 42)
	v, ok := c.Get("key")
	if !ok {
		t.Fatal("expected hit after Add, got miss")
	}
	if v != 42 {
		t.Errorf("Get: got %d, want 42", v)
	}
}

func TestCache_AddReturnsTrueOnEviction(t *testing.T) {
	c, _ := NewCache[int, int](2)
	// First add: no eviction.
	if c.Add(1, 1) {
		t.Error("Add(1) should not evict")
	}
	// Second add: no eviction.
	if c.Add(2, 2) {
		t.Error("Add(2) should not evict")
	}
	// Third add: evicts key 1 (oldest).
	if !c.Add(3, 3) {
		t.Error("Add(3) should evict an entry")
	}
	// Key 1 should have been evicted.
	if _, ok := c.Get(1); ok {
		t.Error("expected key 1 to be evicted")
	}
	// Keys 2 and 3 should still be present.
	if v, ok := c.Get(2); !ok || v != 2 {
		t.Errorf("expected key 2 = 2, got ok=%v v=%v", ok, v)
	}
	if v, ok := c.Get(3); !ok || v != 3 {
		t.Errorf("expected key 3 = 3, got ok=%v v=%v", ok, v)
	}
}

func TestCache_LRU_EvictionOrder(t *testing.T) {
	c, _ := NewCache[int, int](2)
	c.Add(1, 1)
	c.Add(2, 2)
	// Access key 1 so it becomes most-recently-used.
	c.Get(1)
	// Add key 3: should evict the least-recently-used (key 2).
	c.Add(3, 3)
	if _, ok := c.Get(1); !ok {
		t.Error("expected key 1 to survive (was recently accessed)")
	}
	if _, ok := c.Get(2); ok {
		t.Error("expected key 2 to be evicted (LRU)")
	}
	if _, ok := c.Get(3); !ok {
		t.Error("expected key 3 to be present")
	}
}

func TestCache_Remove(t *testing.T) {
	c, _ := NewCache[int, int](10)
	c.Add(1, 1)
	if !c.Remove(1) {
		t.Error("Remove(1) should return true when key exists")
	}
	if _, ok := c.Get(1); ok {
		t.Error("expected key 1 to be removed")
	}
	// Removing a missing key returns false.
	if c.Remove(99) {
		t.Error("Remove(99) should return false for missing key")
	}
}

func TestCache_Len(t *testing.T) {
	c, _ := NewCache[int, int](10)
	if c.Len() != 0 {
		t.Errorf("Len: got %d, want 0", c.Len())
	}
	c.Add(1, 1)
	c.Add(2, 2)
	if c.Len() != 2 {
		t.Errorf("Len: got %d, want 2", c.Len())
	}
	c.Remove(1)
	if c.Len() != 1 {
		t.Errorf("Len: got %d, want 1", c.Len())
	}
}

func TestCache_OverwriteExisting(t *testing.T) {
	c, _ := NewCache[int, int](10)
	c.Add(1, 100)
	c.Add(1, 200) // overwrite
	if c.Len() != 1 {
		t.Errorf("Len after overwrite: got %d, want 1", c.Len())
	}
	v, ok := c.Get(1)
	if !ok || v != 200 {
		t.Errorf("Get after overwrite: got %v %v, want 200 true", v, ok)
	}
}

func TestCache_GenericTypes(t *testing.T) {
	// Verify the generic cache works with string keys and struct values.
	type item struct{ Name string }
	c, _ := NewCache[string, item](5)
	c.Add("a", item{Name: "alpha"})
	v, ok := c.Get("a")
	if !ok || v.Name != "alpha" {
		t.Errorf("Get(string-keyed): got %v %v, want alpha true", v, ok)
	}
}
