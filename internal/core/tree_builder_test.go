package core

import (
	"path"
	"sort"
	"testing"
)

// fakeStore collects trees passed to its store function for later inspection.
type fakeStore struct {
	trees map[string]*Tree
}

func newFakeStore() *fakeStore {
	return &fakeStore{trees: make(map[string]*Tree)}
}

func (f *fakeStore) put(t *Tree) error {
	f.trees[t.Hash] = t
	return nil
}

// TestTreeBuilder_FlatIndex verifies that a flat index produces a single root tree.
func TestTreeBuilder_FlatIndex(t *testing.T) {
	idx := &Index{}
	idx.Add(IndexEntry{Path: "a.txt", Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: ModeRegular})
	idx.Add(IndexEntry{Path: "b.txt", Hash: "1111111111111111111111111111111111111111111111111111111111111111", Mode: ModeRegular})

	store := newFakeStore()
	b := NewTreeBuilder(store.put)
	root, err := b.BuildFromIndex(idx)
	if err != nil {
		t.Fatalf("BuildFromIndex failed: %v", err)
	}
	if len(root.Entries) != 2 {
		t.Fatalf("expected 2 root entries, got %d", len(root.Entries))
	}
	// Root tree should be stored.
	if _, ok := store.trees[root.Hash]; !ok {
		t.Fatal("root tree was not stored")
	}
}

// TestTreeBuilder_NestedIndex verifies that nested paths produce subtrees.
func TestTreeBuilder_NestedIndex(t *testing.T) {
	idx := &Index{}
	idx.Add(IndexEntry{Path: "dir/a.txt", Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: ModeRegular})
	idx.Add(IndexEntry{Path: "dir/sub/b.txt", Hash: "1111111111111111111111111111111111111111111111111111111111111111", Mode: ModeRegular})
	idx.Add(IndexEntry{Path: "c.txt", Hash: "2222222222222222222222222222222222222222222222222222222222222222", Mode: ModeRegular})

	store := newFakeStore()
	b := NewTreeBuilder(store.put)
	root, err := b.BuildFromIndex(idx)
	if err != nil {
		t.Fatalf("BuildFromIndex failed: %v", err)
	}
	if len(root.Entries) != 2 {
		t.Fatalf("expected 2 root entries (dir + c.txt), got %d", len(root.Entries))
	}
	// dir subtree should be stored.
	var dirEntry *TreeEntry
	for i := range root.Entries {
		if root.Entries[i].Name == "dir" {
			dirEntry = &root.Entries[i]
		}
	}
	if dirEntry == nil {
		t.Fatal("expected 'dir' entry in root")
	}
	dirTree, ok := store.trees[dirEntry.Hash]
	if !ok {
		t.Fatal("dir subtree was not stored")
	}
	if len(dirTree.Entries) != 2 {
		t.Fatalf("expected 2 entries in dir subtree, got %d", len(dirTree.Entries))
	}
}

// TestTreeBuilder_DeterministicHash verifies that the same index always produces the same root hash.
func TestTreeBuilder_DeterministicHash(t *testing.T) {
	mk := func() *Index {
		idx := &Index{}
		idx.Add(IndexEntry{Path: "a.txt", Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: ModeRegular})
		idx.Add(IndexEntry{Path: "dir/b.txt", Hash: "1111111111111111111111111111111111111111111111111111111111111111", Mode: ModeRegular})
		return idx
	}
	r1, err := NewTreeBuilder(newFakeStore().put).BuildFromIndex(mk())
	if err != nil {
		t.Fatal(err)
	}
	r2, err := NewTreeBuilder(newFakeStore().put).BuildFromIndex(mk())
	if err != nil {
		t.Fatal(err)
	}
	if r1.Hash != r2.Hash {
		t.Fatalf("deterministic hash mismatch: %q vs %q", r1.Hash, r2.Hash)
	}
}

// TestTreeBuilder_EmptyIndex verifies that an empty index produces an empty root tree.
func TestTreeBuilder_EmptyIndex(t *testing.T) {
	idx := &Index{}
	store := newFakeStore()
	root, err := NewTreeBuilder(store.put).BuildFromIndex(idx)
	if err != nil {
		t.Fatalf("BuildFromIndex failed: %v", err)
	}
	if len(root.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(root.Entries))
	}
}

// TestTreeBuilder_StoresAllSubtrees verifies that every subtree (including the root) is persisted.
func TestTreeBuilder_StoresAllSubtrees(t *testing.T) {
	idx := &Index{}
	idx.Add(IndexEntry{Path: "a/b/c.txt", Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: ModeRegular})
	idx.Add(IndexEntry{Path: "a/d.txt", Hash: "1111111111111111111111111111111111111111111111111111111111111111", Mode: ModeRegular})
	idx.Add(IndexEntry{Path: "e.txt", Hash: "2222222222222222222222222222222222222222222222222222222222222222", Mode: ModeRegular})

	store := newFakeStore()
	root, err := NewTreeBuilder(store.put).BuildFromIndex(idx)
	if err != nil {
		t.Fatalf("BuildFromIndex failed: %v", err)
	}

	// Walk the tree and verify every subtree is in the store.
	visited := map[string]bool{}
	var walk func(hash string) error
	walk = func(hash string) error {
		if visited[hash] {
			return nil
		}
		visited[hash] = true
		t, ok := store.trees[hash]
		if !ok {
			return nil // leaf blob, no tree
		}
		for _, e := range t.Entries {
			if e.Type == TreeObject {
				if err := walk(e.Hash); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root.Hash); err != nil {
		t.Fatalf("walk failed: %v", err)
	}

	// Expected: root, "a", "a/b"
	expected := []string{root.Hash}
	for _, e := range root.Entries {
		if e.Type == TreeObject && e.Name == "a" {
			expected = append(expected, e.Hash)
			aTree := store.trees[e.Hash]
			for _, ae := range aTree.Entries {
				if ae.Type == TreeObject && ae.Name == "b" {
					expected = append(expected, ae.Hash)
				}
			}
		}
	}
	sort.Strings(expected)
	got := make([]string, 0, len(visited))
	for h := range visited {
		got = append(got, h)
	}
	sort.Strings(got)
	if len(got) != len(expected) {
		t.Fatalf("expected %d stored trees, got %d", len(expected), len(got))
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Fatalf("stored tree list mismatch at %d: got %q, want %q", i, got[i], expected[i])
		}
	}
	_ = path.Join // keep import if future expansion needs it
}
