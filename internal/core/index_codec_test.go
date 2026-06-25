package core

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"
)

// TestIndex_MarshalUnmarshal_RoundTrip verifies that an index survives a marshal/unmarshal cycle.
func TestIndex_MarshalUnmarshal_RoundTrip(t *testing.T) {
	idx := &Index{}
	idx.Add(IndexEntry{
		Path:       "dir/a.txt",
		Hash:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ModifiedAt: time.UnixMilli(1700000000123),
		Size:       42,
		Mode:       ModeRegular,
	})
	idx.Add(IndexEntry{
		Path:       "b.bin",
		Hash:       "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
		ModifiedAt: time.UnixMilli(1700000000456),
		Size:       7,
		Mode:       ModeExecutable,
	})

	data, err := idx.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	got := &Index{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(got.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got.Entries))
	}
	for _, want := range idx.Entries {
		e, err := got.Entry(want.Path)
		if err != nil {
			t.Fatalf("missing entry %q after round trip: %v", want.Path, err)
		}
		if e.Hash != want.Hash || e.Size != want.Size || e.Mode != want.Mode {
			t.Fatalf("entry %q mismatch: got %+v, want %+v", want.Path, e, want)
		}
		if !e.ModifiedAt.Equal(want.ModifiedAt) {
			t.Fatalf("entry %q mtime mismatch: got %v, want %v", want.Path, e.ModifiedAt, want.ModifiedAt)
		}
	}
}

// withChecksum appends a valid SHA-256 checksum trailer to the body.
func withChecksum(body []byte) []byte {
	h := sha256.Sum256(body)
	return append(body, h[:]...)
}

// TestIndex_Unmarshal_BadMagic verifies that an invalid magic header is rejected.
func TestIndex_Unmarshal_BadMagic(t *testing.T) {
	body := []byte{'X', 'X', 'X', 'X', 1, 0, 0, 0, 0, 0, 0, 0}
	data := withChecksum(body)
	idx := &Index{}
	if err := idx.Unmarshal(data); err != ErrInvalidIndex {
		t.Fatalf("expected ErrInvalidIndex, got %v", err)
	}
}

// TestIndex_Unmarshal_UnsupportedVersion verifies that an unsupported version is rejected.
func TestIndex_Unmarshal_UnsupportedVersion(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(indexMagic[:])
	buf.Write([]byte{99, 0, 0, 0}) // version 99
	buf.Write([]byte{0, 0, 0, 0})  // count 0
	data := withChecksum(buf.Bytes())
	idx := &Index{}
	if err := idx.Unmarshal(data); err != ErrIndexVersion {
		t.Fatalf("expected ErrIndexVersion, got %v", err)
	}
}

// TestIndex_Unmarshal_Truncated verifies that a truncated body is reported as corrupt.
func TestIndex_Unmarshal_Truncated(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(indexMagic[:])
	buf.Write([]byte{1, 0, 0, 0}) // version 1
	buf.Write([]byte{1, 0, 0, 0}) // count 1
	buf.Write([]byte{5, 0})       // path length 5
	buf.Write([]byte("a.txt"))    // path
	// Missing hash, timestamp, size, mode.
	data := withChecksum(buf.Bytes())
	idx := &Index{}
	if err := idx.Unmarshal(data); err != ErrIndexCorrupt {
		t.Fatalf("expected ErrIndexCorrupt, got %v", err)
	}
}

// TestIndex_Marshal_InvalidHash verifies that an invalid hex hash is rejected at marshal time.
func TestIndex_Marshal_InvalidHash(t *testing.T) {
	idx := &Index{}
	idx.Add(IndexEntry{Path: "a", Hash: "not-hex", Mode: ModeRegular})
	if _, err := idx.Marshal(); err == nil {
		t.Fatal("expected error for invalid hash, got nil")
	}
}

// TestIndex_Marshal_PathTooLong verifies that a path longer than 65535 bytes is rejected.
func TestIndex_Marshal_PathTooLong(t *testing.T) {
	idx := &Index{}
	long := make([]byte, 70000)
	for i := range long {
		long[i] = 'a'
	}
	idx.Add(IndexEntry{
		Path: string(long),
		Hash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Mode: ModeRegular,
	})
	if _, err := idx.Marshal(); err == nil {
		t.Fatal("expected error for overlong path, got nil")
	}
}

// TestIndex_MarshalUnmarshal_Empty verifies that an empty index round-trips correctly.
func TestIndex_MarshalUnmarshal_Empty(t *testing.T) {
	idx := &Index{}
	data, err := idx.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	got := &Index{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(got.Entries))
	}
}
