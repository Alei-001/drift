package core

import (
	"errors"
	"sort"
	"testing"
)

// memoryStore is a minimal StoreReader for testing tree walking.
type memoryStore struct {
	trees map[string]*Tree
	blobs map[string][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		trees: make(map[string]*Tree),
		blobs: make(map[string][]byte),
	}
}

var errTestNotFound = errors.New("object not found")

func (m *memoryStore) GetTree(hash string) (*Tree, error) {
	t, ok := m.trees[hash]
	if !ok {
		return nil, errTestNotFound
	}
	return t, nil
}

func (m *memoryStore) GetBlob(hash string) ([]byte, error) {
	b, ok := m.blobs[hash]
	if !ok {
		return nil, errTestNotFound
	}
	return b, nil
}

// buildNestedStore builds a store containing:
//
//	root
//	├── a.txt
//	└── dir
//	    ├── b.txt
//	    └── sub
//	        └── c.txt
func buildNestedStore(t *testing.T) (*memoryStore, *Tree) {
	t.Helper()
	store := newMemoryStore()

	mkBlob := func(name, content string) TreeEntry {
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		return TreeEntry{Name: name, Type: BlobObject, Hash: hash, Mode: ModeRegular}
	}

	subTree, err := NewTree([]TreeEntry{mkBlob("c.txt", "c")})
	if err != nil {
		t.Fatal(err)
	}
	store.trees[subTree.Hash] = subTree

	dirTree, err := NewTree([]TreeEntry{
		mkBlob("b.txt", "b"),
		{Name: "sub", Type: TreeObject, Hash: subTree.Hash, Mode: ModeDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.trees[dirTree.Hash] = dirTree

	root, err := NewTree([]TreeEntry{
		mkBlob("a.txt", "a"),
		{Name: "dir", Type: TreeObject, Hash: dirTree.Hash, Mode: ModeDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.trees[root.Hash] = root
	return store, root
}

// TestTreeReader_ListBlobs_Recursive verifies that ListBlobs walks the entire tree.
func TestTreeReader_ListBlobs_Recursive(t *testing.T) {
	store, root := buildNestedStore(t)
	reader := NewTreeReader(store)

	blobs, err := reader.ListBlobs(root, "")
	if err != nil {
		t.Fatalf("ListBlobs failed: %v", err)
	}
	paths := make([]string, 0, len(blobs))
	for _, b := range blobs {
		paths = append(paths, b.Path)
	}
	sort.Strings(paths)
	want := []string{"a.txt", "dir/b.txt", "dir/sub/c.txt"}
	if len(paths) != len(want) {
		t.Fatalf("expected %d blobs, got %d (%v)", len(want), len(paths), paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("blob %d: got %q, want %q", i, paths[i], want[i])
		}
	}
}

// TestTreeReader_ListBlobs_Prefix verifies that the prefix is prepended to paths.
func TestTreeReader_ListBlobs_Prefix(t *testing.T) {
	store, root := buildNestedStore(t)
	reader := NewTreeReader(store)

	blobs, err := reader.ListBlobs(root, "root")
	if err != nil {
		t.Fatalf("ListBlobs failed: %v", err)
	}
	expected := map[string]bool{"root/a.txt": true, "root/dir/b.txt": true, "root/dir/sub/c.txt": true}
	got := make(map[string]bool)
	for _, b := range blobs {
		got[b.Path] = true
	}
	for p := range expected {
		if !got[p] {
			t.Fatalf("expected blob %q, not found in %v", p, got)
		}
	}
}

// TestTreeReader_ListBlobs_MissingSubtree verifies that a missing subtree returns an error.
func TestTreeReader_ListBlobs_MissingSubtree(t *testing.T) {
	store := newMemoryStore()
	root, err := NewTree([]TreeEntry{
		{Name: "missing", Type: TreeObject, Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: ModeDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewTreeReader(store)
	if _, err := reader.ListBlobs(root, ""); err == nil {
		t.Fatal("expected error for missing subtree, got nil")
	}
}

// TestTreeReader_DiffTrees verifies that DiffTrees correctly classifies deleted/added/modified blobs.
func TestTreeReader_DiffTrees(t *testing.T) {
	store := newMemoryStore()

	mkBlob := func(name, content string) TreeEntry {
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		return TreeEntry{Name: name, Type: BlobObject, Hash: hash, Mode: ModeRegular}
	}

	oldTree, err := NewTree([]TreeEntry{
		mkBlob("keep.txt", "k"),
		mkBlob("del.txt", "d"),
		mkBlob("mod.txt", "old"),
	})
	if err != nil {
		t.Fatal(err)
	}
	newTree, err := NewTree([]TreeEntry{
		mkBlob("keep.txt", "k"),
		mkBlob("mod.txt", "new"),
		mkBlob("add.txt", "a"),
	})
	if err != nil {
		t.Fatal(err)
	}

	reader := NewTreeReader(store)
	deleted, added, modified, err := reader.DiffTrees(oldTree, newTree)
	if err != nil {
		t.Fatalf("DiffTrees failed: %v", err)
	}
	if len(deleted) != 1 || deleted[0].Path != "del.txt" {
		t.Fatalf("deleted = %+v, want [del.txt]", deleted)
	}
	if len(added) != 1 || added[0].Path != "add.txt" {
		t.Fatalf("added = %+v, want [add.txt]", added)
	}
	if len(modified) != 1 || modified[0].Path != "mod.txt" {
		t.Fatalf("modified = %+v, want [mod.txt]", modified)
	}
}

// TestTreeReader_DiffTrees_Identical verifies that identical trees produce no diffs.
func TestTreeReader_DiffTrees_Identical(t *testing.T) {
	store := newMemoryStore()
	mkBlob := func(name, content string) TreeEntry {
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		return TreeEntry{Name: name, Type: BlobObject, Hash: hash, Mode: ModeRegular}
	}
	tree, err := NewTree([]TreeEntry{mkBlob("a.txt", "a")})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewTreeReader(store)
	deleted, added, modified, err := reader.DiffTrees(tree, tree)
	if err != nil {
		t.Fatalf("DiffTrees failed: %v", err)
	}
	if len(deleted) != 0 || len(added) != 0 || len(modified) != 0 {
		t.Fatalf("expected no diffs, got deleted=%d added=%d modified=%d", len(deleted), len(added), len(modified))
	}
}
