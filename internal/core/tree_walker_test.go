package core

import (
	"errors"
	"sort"
	"testing"
)

// memoryStore is a minimal StoreReader for testing tree walking.
type memoryStore struct {
	trees   map[string]*Tree
	blobs   map[string][]byte
	commits map[string]*Commit
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		trees:   make(map[string]*Tree),
		blobs:   make(map[string][]byte),
		commits: make(map[string]*Commit),
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

func (m *memoryStore) GetBlobSize(hash string) (int64, error) {
	b, ok := m.blobs[hash]
	if !ok {
		return 0, errTestNotFound
	}
	return int64(len(b)), nil
}

func (m *memoryStore) GetCommit(hash string) (*Commit, error) {
	c, ok := m.commits[hash]
	if !ok {
		return nil, errTestNotFound
	}
	return c, nil
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
	// Construct tree directly to bypass Validate (which rejects null hashes);
	// this test specifically exercises the reader's missing-subtree path.
	root := &Tree{Entries: []TreeEntry{
		{Name: "missing", Type: TreeObject, Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: ModeDir},
	}}
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

// B1: Merkletrie lazy diff tests

func TestLazyDiffTrees_Identical(t *testing.T) {
	store := newMemoryStore()
	mkBlob := func(name, content string) TreeEntry {
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		return TreeEntry{Name: name, Type: BlobObject, Hash: hash, Mode: ModeRegular}
	}
	tree, err := NewTree([]TreeEntry{mkBlob("a.txt", "a"), mkBlob("b.txt", "b")})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewTreeReader(store)
	changes, err := reader.LazyDiffTrees(tree, tree)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %d", len(changes))
	}
}

func TestLazyDiffTrees_Added(t *testing.T) {
	store := newMemoryStore()
	mkBlob := func(name, content string) TreeEntry {
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		return TreeEntry{Name: name, Type: BlobObject, Hash: hash, Mode: ModeRegular}
	}
	oldTree, err := NewTree([]TreeEntry{mkBlob("a.txt", "a")})
	if err != nil {
		t.Fatal(err)
	}
	newTree, err := NewTree([]TreeEntry{mkBlob("a.txt", "a"), mkBlob("b.txt", "b")})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewTreeReader(store)
	changes, err := reader.LazyDiffTrees(oldTree, newTree)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].Old != nil || changes[0].New == nil || changes[0].New.Path != "b.txt" {
		t.Fatalf("expected added b.txt, got %+v", changes[0])
	}
}

func TestLazyDiffTrees_Deleted(t *testing.T) {
	store := newMemoryStore()
	mkBlob := func(name, content string) TreeEntry {
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		return TreeEntry{Name: name, Type: BlobObject, Hash: hash, Mode: ModeRegular}
	}
	oldTree, err := NewTree([]TreeEntry{mkBlob("a.txt", "a"), mkBlob("b.txt", "b")})
	if err != nil {
		t.Fatal(err)
	}
	newTree, err := NewTree([]TreeEntry{mkBlob("a.txt", "a")})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewTreeReader(store)
	changes, err := reader.LazyDiffTrees(oldTree, newTree)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %+v", len(changes), changes)
	}
	if changes[0].New != nil || changes[0].Old == nil || changes[0].Old.Path != "b.txt" {
		t.Fatalf("expected deleted b.txt, got %+v", changes[0])
	}
}

func TestLazyDiffTrees_Modified(t *testing.T) {
	store := newMemoryStore()
	mkBlob := func(name, content string) TreeEntry {
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		return TreeEntry{Name: name, Type: BlobObject, Hash: hash, Mode: ModeRegular}
	}
	oldTree, err := NewTree([]TreeEntry{mkBlob("a.txt", "old")})
	if err != nil {
		t.Fatal(err)
	}
	newTree, err := NewTree([]TreeEntry{mkBlob("a.txt", "new")})
	if err != nil {
		t.Fatal(err)
	}
	reader := NewTreeReader(store)
	changes, err := reader.LazyDiffTrees(oldTree, newTree)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Old == nil || changes[0].New == nil || changes[0].Old.Hash == changes[0].New.Hash {
		t.Fatalf("expected modified a.txt, got %+v", changes[0])
	}
}

func TestLazyDiffTrees_NestedDir_SameHash_Skips(t *testing.T) {
	store := newMemoryStore()

	// Build a subtree (dir/) with two files.
	subEntries := []TreeEntry{
		{Name: "x.txt", Type: BlobObject, Hash: CalculateHash([]byte("x")), Mode: ModeRegular},
		{Name: "y.txt", Type: BlobObject, Hash: CalculateHash([]byte("y")), Mode: ModeRegular},
	}
	store.blobs[CalculateHash([]byte("x"))] = []byte("x")
	store.blobs[CalculateHash([]byte("y"))] = []byte("y")

	subTree, err := NewTree(subEntries)
	if err != nil {
		t.Fatal(err)
	}
	store.trees[subTree.Hash] = subTree

	// Root trees contain the same dir entry (same hash) and a modified root file.
	oldHash := CalculateHash([]byte("old"))
	newHash := CalculateHash([]byte("new"))
	store.blobs[oldHash] = []byte("old")
	store.blobs[newHash] = []byte("new")

	oldRoot, err := NewTree([]TreeEntry{
		{Name: "dir", Type: TreeObject, Hash: subTree.Hash, Mode: ModeDir},
		{Name: "z.txt", Type: BlobObject, Hash: oldHash, Mode: ModeRegular},
	})
	if err != nil {
		t.Fatal(err)
	}
	newRoot, err := NewTree([]TreeEntry{
		{Name: "dir", Type: TreeObject, Hash: subTree.Hash, Mode: ModeDir},
		{Name: "z.txt", Type: BlobObject, Hash: newHash, Mode: ModeRegular},
	})
	if err != nil {
		t.Fatal(err)
	}

	reader := NewTreeReader(store)
	changes, err := reader.LazyDiffTrees(oldRoot, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	// Only z.txt should show as modified — dir/ subtree is skipped.
	if len(changes) != 1 || changes[0].Path != "z.txt" {
		t.Fatalf("expected 1 change (z.txt), got %d: %+v", len(changes), changes)
	}
}

func TestLazyDiffTrees_NestedDir_Modified(t *testing.T) {
	store := newMemoryStore()

	// Build old subtree dir/ with x.txt.
	oldSubEntries := []TreeEntry{
		{Name: "x.txt", Type: BlobObject, Hash: CalculateHash([]byte("old")), Mode: ModeRegular},
	}
	store.blobs[CalculateHash([]byte("old"))] = []byte("old")
	oldSub, err := NewTree(oldSubEntries)
	if err != nil {
		t.Fatal(err)
	}
	store.trees[oldSub.Hash] = oldSub

	// Build new subtree dir/ with modified x.txt.
	newSubEntries := []TreeEntry{
		{Name: "x.txt", Type: BlobObject, Hash: CalculateHash([]byte("new")), Mode: ModeRegular},
	}
	store.blobs[CalculateHash([]byte("new"))] = []byte("new")
	newSub, err := NewTree(newSubEntries)
	if err != nil {
		t.Fatal(err)
	}
	store.trees[newSub.Hash] = newSub

	oldRoot, err := NewTree([]TreeEntry{
		{Name: "dir", Type: TreeObject, Hash: oldSub.Hash, Mode: ModeDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	newRoot, err := NewTree([]TreeEntry{
		{Name: "dir", Type: TreeObject, Hash: newSub.Hash, Mode: ModeDir},
	})
	if err != nil {
		t.Fatal(err)
	}

	reader := NewTreeReader(store)
	changes, err := reader.LazyDiffTrees(oldRoot, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Path != "dir/x.txt" {
		t.Fatalf("expected 1 change (dir/x.txt), got %d: %+v", len(changes), changes)
	}
}
