package core

import (
	"bytes"
	"testing"
)

// TestNewTree_SortsEntries verifies that NewTree sorts entries (dirs first, then by name).
func TestNewTree_SortsEntries(t *testing.T) {
	entries := []TreeEntry{
		{Name: "z.txt", Type: BlobObject, Hash: "3333333333333333333333333333333333333333333333333333333333333333", Mode: ModeRegular},
		{Name: "sub", Type: TreeObject, Hash: "1111111111111111111111111111111111111111111111111111111111111111", Mode: ModeDir},
		{Name: "a.txt", Type: BlobObject, Hash: "2222222222222222222222222222222222222222222222222222222222222222", Mode: ModeRegular},
	}
	tree, err := NewTree(entries)
	if err != nil {
		t.Fatalf("NewTree failed: %v", err)
	}
	if len(tree.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(tree.Entries))
	}
	if tree.Entries[0].Name != "a.txt" {
		t.Fatalf("expected a.txt first, got %q", tree.Entries[0].Name)
	}
	if tree.Entries[1].Name != "sub" {
		t.Fatalf("expected sub second, got %q", tree.Entries[1].Name)
	}
	if tree.Entries[2].Name != "z.txt" {
		t.Fatalf("expected z.txt third, got %q", tree.Entries[2].Name)
	}
}

// TestNewTree_DeterministicHash verifies that two trees with the same entries (in any order) hash identically.
func TestNewTree_DeterministicHash(t *testing.T) {
	mk := func() []TreeEntry {
		return []TreeEntry{
			{Name: "a", Type: BlobObject, Hash: "0000000000000000000000000000000000000000000000000000000000000001", Mode: ModeRegular},
			{Name: "b", Type: BlobObject, Hash: "1111111111111111111111111111111111111111111111111111111111111111", Mode: ModeRegular},
		}
	}
	t1, _ := NewTree(mk())
	t2, _ := NewTree([]TreeEntry{
		{Name: "b", Type: BlobObject, Hash: "1111111111111111111111111111111111111111111111111111111111111111", Mode: ModeRegular},
		{Name: "a", Type: BlobObject, Hash: "0000000000000000000000000000000000000000000000000000000000000001", Mode: ModeRegular},
	})
	if t1.Hash != t2.Hash {
		t.Fatalf("deterministic hash mismatch: %q vs %q", t1.Hash, t2.Hash)
	}
}

// TestTree_MarshalUnmarshal_RoundTrip verifies DREE round trip.
func TestTree_MarshalUnmarshal_RoundTrip(t *testing.T) {
	entries := []TreeEntry{
		{Name: "sub", Type: TreeObject, Hash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Mode: ModeDir},
		{Name: "a.txt", Type: BlobObject, Hash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", Mode: ModeRegular},
		{Name: "run.sh", Type: BlobObject, Hash: "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210", Mode: ModeExecutable},
	}
	tree, err := NewTree(entries)
	if err != nil {
		t.Fatalf("NewTree failed: %v", err)
	}

	data, err := tree.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	got := &Tree{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(got.Entries) != len(tree.Entries) {
		t.Fatalf("entry count mismatch: got %d, want %d", len(got.Entries), len(tree.Entries))
	}
	for i := range tree.Entries {
		if got.Entries[i] != tree.Entries[i] {
			t.Fatalf("entry %d mismatch: got %+v, want %+v", i, got.Entries[i], tree.Entries[i])
		}
	}
}

// TestTree_Unmarshal_BadMagic verifies invalid magic is rejected.
func TestTree_Unmarshal_BadMagic(t *testing.T) {
	data := []byte{'X', 'X', 'X', 'X', 0, 0, 0, 0}
	tree := &Tree{}
	if err := tree.Unmarshal(data); err != ErrInvalidTree {
		t.Fatalf("expected ErrInvalidTree, got %v", err)
	}
}

// TestTree_Unmarshal_Truncated verifies truncated body is rejected.
func TestTree_Unmarshal_Truncated(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(treeMagic[:])
	buf.Write([]byte{1, 0, 0, 0}) // count 1
	buf.Write([]byte{3, 0})       // name length 3
	buf.Write([]byte("abc"))      // name
	// Missing type, hash, mode.
	tree := &Tree{}
	if err := tree.Unmarshal(buf.Bytes()); err != ErrTreeCorrupt {
		t.Fatalf("expected ErrTreeCorrupt, got %v", err)
	}
}

// TestTree_Marshal_InvalidHash verifies invalid hex hash is rejected.
func TestTree_Marshal_InvalidHash(t *testing.T) {
	tree := &Tree{Entries: []TreeEntry{{Name: "a", Type: BlobObject, Hash: "not-hex", Mode: ModeRegular}}}
	if _, err := tree.Marshal(); err == nil {
		t.Fatal("expected error for invalid hash, got nil")
	}
}
