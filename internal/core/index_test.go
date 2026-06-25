package core

import (
	"errors"
	"testing"
	"time"
)

func sampleEntry(path string) IndexEntry {
	return IndexEntry{
		Path:       path,
		Hash:       CalculateHash([]byte(path)),
		ModifiedAt: time.UnixMilli(1700000000),
		Size:       int64(len(path)),
		Mode:       ModeRegular,
	}
}

// TestIndex_Entry_NotFound verifies that looking up a missing entry returns ErrEntryNotFound.
func TestIndex_Entry_NotFound(t *testing.T) {
	idx := &Index{}
	if _, err := idx.Entry("missing"); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("expected ErrEntryNotFound, got %v", err)
	}
}

// TestIndex_Add_Then_Entry verifies add then lookup returns the stored entry.
func TestIndex_Add_Then_Entry(t *testing.T) {
	idx := &Index{}
	e := sampleEntry("a.txt")
	idx.Add(e)

	got, err := idx.Entry("a.txt")
	if err != nil {
		t.Fatalf("Entry returned error: %v", err)
	}
	if got.Path != "a.txt" {
		t.Fatalf("got path %q, want a.txt", got.Path)
	}
	if got.Hash != e.Hash {
		t.Fatalf("hash mismatch")
	}
}

// TestIndex_Add_Replace verifies that adding the same path replaces the existing entry.
func TestIndex_Add_Replace(t *testing.T) {
	idx := &Index{}
	idx.Add(sampleEntry("a.txt"))
	newEntry := IndexEntry{Path: "a.txt", Hash: "newhash", Mode: ModeRegular}
	idx.Add(newEntry)

	got, _ := idx.Entry("a.txt")
	if got.Hash != "newhash" {
		t.Fatalf("expected replaced hash, got %q", got.Hash)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("expected 1 entry after replace, got %d", len(idx.Entries))
	}
}

// TestIndex_Has verifies Has returns true only for added entries.
func TestIndex_Has(t *testing.T) {
	idx := &Index{}
	idx.Add(sampleEntry("a.txt"))
	if !idx.Has("a.txt") {
		t.Fatal("Has(a.txt) = false, want true")
	}
	if idx.Has("b.txt") {
		t.Fatal("Has(b.txt) = true, want false")
	}
}

// TestIndex_Remove verifies removal makes the entry inaccessible.
func TestIndex_Remove(t *testing.T) {
	idx := &Index{}
	idx.Add(sampleEntry("a.txt"))
	idx.Add(sampleEntry("b.txt"))

	idx.Remove("a.txt")
	if idx.Has("a.txt") {
		t.Fatal("Has(a.txt) = true after Remove")
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(idx.Entries))
	}
	if !idx.Has("b.txt") {
		t.Fatal("Has(b.txt) = false, expected true")
	}
}

// TestIndex_Remove_Missing is a no-op on a missing path.
func TestIndex_Remove_Missing(t *testing.T) {
	idx := &Index{}
	idx.Add(sampleEntry("a.txt"))
	before := len(idx.Entries)
	idx.Remove("missing")
	if len(idx.Entries) != before {
		t.Fatalf("Remove(missing) changed entry count from %d to %d", before, len(idx.Entries))
	}
}

// TestIndex_Clear resets the index to empty.
func TestIndex_Clear(t *testing.T) {
	idx := &Index{}
	idx.Add(sampleEntry("a.txt"))
	idx.Add(sampleEntry("b.txt"))

	idx.Clear()
	if len(idx.Entries) != 0 {
		t.Fatalf("expected 0 entries after Clear, got %d", len(idx.Entries))
	}
	if idx.Has("a.txt") {
		t.Fatal("Has(a.txt) = true after Clear")
	}
}

// TestIndex_BuildIndex_AfterManualAppend verifies that an externally-built index
// (e.g. after Unmarshal) is searchable after the byPath map is lazily built.
func TestIndex_BuildIndex_AfterManualAppend(t *testing.T) {
	idx := &Index{}
	idx.Entries = append(idx.Entries, sampleEntry("a.txt"))
	// byPath is nil; Entry should build it lazily.
	got, err := idx.Entry("a.txt")
	if err != nil {
		t.Fatalf("Entry returned error: %v", err)
	}
	if got.Path != "a.txt" {
		t.Fatalf("got path %q, want a.txt", got.Path)
	}
}
