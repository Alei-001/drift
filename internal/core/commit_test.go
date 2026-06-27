package core

import (
	"bytes"
	"testing"
	"time"
)

// TestNewCommit_AssignsFields verifies that NewCommit populates all fields,
// computes a hash, and sets ID to the hash prefix.
func TestNewCommit_AssignsFields(t *testing.T) {
	author := Signature{Name: "alice", Email: "alice@example.com"}
	c, err := NewCommit("msg", "", "main", "treehash", author)
	if err != nil {
		t.Fatal(err)
	}
	if c.Message != "msg" || c.Branch != "main" || c.TreeHash != "treehash" {
		t.Fatalf("fields not assigned: %+v", c)
	}
	if c.Hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if c.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if c.ID != c.Hash[:CommitIDLen] {
		t.Fatalf("ID should be hash prefix: got %q, want %q", c.ID, c.Hash[:CommitIDLen])
	}
	if c.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

// TestCommit_isRoot verifies root detection logic.
func TestCommit_isRoot(t *testing.T) {
	root, err := NewCommit("m", "", "main", "t", Signature{})
	if err != nil {
		t.Fatal(err)
	}
	if !root.isRoot() {
		t.Fatal("commit with empty parent should be root")
	}
	nullParent, err := NewCommit("m", nullHash, "main", "t", Signature{})
	if err != nil {
		t.Fatal(err)
	}
	if !nullParent.isRoot() {
		t.Fatal("commit with null-hash parent should be root")
	}
	nonRoot, err := NewCommit("m", "abcd", "main", "t", Signature{})
	if err != nil {
		t.Fatal(err)
	}
	if nonRoot.isRoot() {
		t.Fatal("commit with non-empty parent should not be root")
	}
}

// TestCommit_MarshalUnmarshal_RoundTrip verifies DCMT round trip preserves all fields.
func TestCommit_MarshalUnmarshal_RoundTrip(t *testing.T) {
	author := Signature{Name: "alice", Email: "alice@example.com"}
	c, err := NewCommit("hello world", "", "main", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", author)
	if err != nil {
		t.Fatal(err)
	}
	// Pin timestamp for deterministic comparison.
	c.Timestamp = time.UnixMilli(1700000000000)
	c.Hash = c.calculateHash()
	c.ID = c.Hash[:CommitIDLen]

	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	got := &Commit{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.ID != c.ID || got.Message != c.Message || got.Branch != c.Branch {
		t.Fatalf("fields mismatch: got %+v, want %+v", got, c)
	}
	if got.TreeHash != c.TreeHash {
		t.Fatalf("tree hash mismatch: got %q, want %q", got.TreeHash, c.TreeHash)
	}
	if got.Hash != c.Hash {
		t.Fatalf("hash mismatch: got %q, want %q", got.Hash, c.Hash)
	}
	if got.Parent != c.Parent {
		t.Fatalf("parent mismatch: got %q, want %q", got.Parent, c.Parent)
	}
	if !got.Timestamp.Equal(c.Timestamp) {
		t.Fatalf("timestamp mismatch: got %v, want %v", got.Timestamp, c.Timestamp)
	}
	if got.Author != c.Author {
		t.Fatalf("author mismatch: got %+v, want %+v", got.Author, c.Author)
	}
}

// TestCommit_MarshalUnmarshal_NonRoot verifies that a non-root commit's parent survives round trip.
func TestCommit_MarshalUnmarshal_NonRoot(t *testing.T) {
	parent := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	c, err := NewCommit("msg", parent, "feature", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Signature{Name: "bob", Email: "bob@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	c.Timestamp = time.UnixMilli(1700000000123)
	c.Hash = c.calculateHash()
	c.ID = c.Hash[:CommitIDLen]

	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	got := &Commit{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.Parent != parent {
		t.Fatalf("parent mismatch: got %q, want %q", got.Parent, parent)
	}
}

// TestCommit_Unmarshal_BadMagic verifies invalid magic is rejected.
func TestCommit_Unmarshal_BadMagic(t *testing.T) {
	data := []byte{'X', 'X', 'X', 'X', 1, 0, 0, 0}
	c := &Commit{}
	if err := c.Unmarshal(data); err != ErrInvalidCommit {
		t.Fatalf("expected ErrInvalidCommit, got %v", err)
	}
}

// TestCommit_Unmarshal_BadVersion verifies an unsupported version is rejected.
func TestCommit_Unmarshal_BadVersion(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(commitMagic[:])
	buf.Write([]byte{99, 0, 0, 0}) // version 99
	c := &Commit{}
	if err := c.Unmarshal(buf.Bytes()); err != ErrInvalidCommit {
		t.Fatalf("expected ErrInvalidCommit, got %v", err)
	}
}

// TestCommit_Unmarshal_HashMismatch verifies that tampered commit data is rejected.
func TestCommit_Unmarshal_HashMismatch(t *testing.T) {
	c, err := NewCommit("msg", "", "main", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Signature{Name: "a", Email: "b"})
	if err != nil {
		t.Fatal(err)
	}
	c.Timestamp = time.UnixMilli(1700000000000)
	c.Hash = c.calculateHash()
	c.ID = c.Hash[:CommitIDLen]

	data, err := c.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	// Tamper with the message string (last bytes of the payload).
	data[len(data)-1] ^= 0xFF
	got := &Commit{}
	if err := got.Unmarshal(data); err != ErrCommitHashMismatch {
		t.Fatalf("expected ErrCommitHashMismatch, got %v", err)
	}
}
