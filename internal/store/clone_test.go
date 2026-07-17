package store

import (
	"testing"

	"github.com/Alei-001/drift/internal/core"
)

func TestCloneChunk_Nil(t *testing.T) {
	if got := CloneChunk(nil); got != nil {
		t.Errorf("CloneChunk(nil) = %v, want nil", got)
	}
}

func TestCloneChunk_DeepCopy(t *testing.T) {
	original := &core.Chunk{
		Hash:  core.Hash{0x01, 0x02},
		Size:  100,
		Data:  []byte("original data"),
		Flags: core.ChunkFlagCompressed,
	}
	clone := CloneChunk(original)
	if clone == nil {
		t.Fatal("expected non-nil clone")
	}
	if clone.Hash != original.Hash {
		t.Errorf("Hash: got %v, want %v", clone.Hash, original.Hash)
	}
	if clone.Size != original.Size {
		t.Errorf("Size: got %d, want %d", clone.Size, original.Size)
	}
	if clone.Flags != original.Flags {
		t.Errorf("Flags: got %d, want %d", clone.Flags, original.Flags)
	}
	if string(clone.Data) != "original data" {
		t.Errorf("Data: got %q, want %q", clone.Data, "original data")
	}

	// Mutate the clone's data — original should be unaffected.
	clone.Data[0] = 'X'
	if original.Data[0] != 'o' {
		t.Error("mutating clone affected original Data")
	}
}

func TestCloneChunk_NilData(t *testing.T) {
	original := &core.Chunk{Hash: core.Hash{0x01}, Data: nil}
	clone := CloneChunk(original)
	if clone.Data != nil {
		t.Errorf("Data: got %v, want nil", clone.Data)
	}
}

func TestCloneFileEntry_DeepCopy(t *testing.T) {
	original := core.FileEntry{
		Path:    "a.txt",
		Mode:    core.FileModeRegular,
		Size:    100,
		Chunks:  []core.Hash{{0x11}, {0x22}},
		Hash:    core.Hash{0xab},
		ModTime: 42,
		Metadata: &core.FileMetadata{
			MIMEType: "text/plain",
			Extra:    map[string]string{"k1": "v1", "k2": "v2"},
		},
	}
	clone := CloneFileEntry(original)
	if clone.Path != original.Path {
		t.Errorf("Path: got %q, want %q", clone.Path, original.Path)
	}
	if clone.Size != original.Size {
		t.Errorf("Size: got %d, want %d", clone.Size, original.Size)
	}
	if len(clone.Chunks) != 2 {
		t.Fatalf("Chunks len: got %d, want 2", len(clone.Chunks))
	}

	// Mutate clone's slices/maps — original should be unaffected.
	clone.Chunks[0] = core.Hash{0xff}
	if original.Chunks[0] != (core.Hash{0x11}) {
		t.Error("mutating clone.Chunks affected original")
	}
	clone.Metadata.Extra["k1"] = "mutated"
	if original.Metadata.Extra["k1"] != "v1" {
		t.Error("mutating clone.Metadata.Extra affected original")
	}
}

func TestCloneFileEntry_NilMetadata(t *testing.T) {
	original := core.FileEntry{Path: "a.txt"}
	clone := CloneFileEntry(original)
	if clone.Metadata != nil {
		t.Errorf("Metadata: got %v, want nil", clone.Metadata)
	}
}

func TestCloneFileEntry_NilChunks(t *testing.T) {
	original := core.FileEntry{Path: "a.txt"}
	clone := CloneFileEntry(original)
	if clone.Chunks != nil {
		t.Errorf("Chunks: got %v, want nil", clone.Chunks)
	}
}

func TestCloneSnapshot_Nil(t *testing.T) {
	if got := CloneSnapshot(nil); got != nil {
		t.Errorf("CloneSnapshot(nil) = %v, want nil", got)
	}
}

func TestCloneSnapshot_DeepCopy(t *testing.T) {
	prevID := core.SnapshotID{Hash: core.Hash{0x99}}
	original := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{0x01}},
		PrevID:    &prevID,
		Message:   "msg",
		Author:    "author",
		Timestamp: 42,
		TotalSize: 1024,
		Files: []core.FileEntry{
			{Path: "a.txt", Chunks: []core.Hash{{0x11}}},
			{Path: "b.txt", Chunks: []core.Hash{{0x22}}},
		},
		Tags: []string{"v1", "v2"},
	}
	clone := CloneSnapshot(original)
	if clone == nil {
		t.Fatal("expected non-nil clone")
	}
	if clone.Message != original.Message {
		t.Errorf("Message: got %q, want %q", clone.Message, original.Message)
	}
	if clone.PrevID == nil || clone.PrevID.Hash != original.PrevID.Hash {
		t.Error("PrevID mismatch")
	}
	if len(clone.Files) != 2 {
		t.Fatalf("Files len: got %d, want 2", len(clone.Files))
	}

	// Mutate clone's slices — original should be unaffected.
	clone.Files[0].Chunks[0] = core.Hash{0xff}
	if original.Files[0].Chunks[0] != (core.Hash{0x11}) {
		t.Error("mutating clone.Files affected original")
	}
	clone.Tags[0] = "mutated"
	if original.Tags[0] != "v1" {
		t.Error("mutating clone.Tags affected original")
	}

	// Mutating clone.PrevID should not affect original.PrevID.
	clone.PrevID.Hash = core.Hash{0xee}
	if original.PrevID.Hash != (core.Hash{0x99}) {
		t.Error("mutating clone.PrevID affected original")
	}
}

func TestCloneSnapshot_NoPrevID(t *testing.T) {
	original := &core.Snapshot{Message: "first"}
	clone := CloneSnapshot(original)
	if clone.PrevID != nil {
		t.Errorf("PrevID: got %v, want nil", clone.PrevID)
	}
}

func TestCloneSnapshot_EmptyFiles(t *testing.T) {
	original := &core.Snapshot{Message: "empty"}
	clone := CloneSnapshot(original)
	if clone.Files != nil {
		t.Errorf("Files: got %v, want nil", clone.Files)
	}
	if clone.Tags != nil {
		t.Errorf("Tags: got %v, want nil", clone.Tags)
	}
}
