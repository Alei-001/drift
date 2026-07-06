package core

import (
	"bytes"
	"testing"
)

func TestSnapshotToProto_Nil(t *testing.T) {
	if got := SnapshotToProto(nil, false); got != nil {
		t.Errorf("SnapshotToProto(nil, false) = %v, want nil", got)
	}
	if got := SnapshotToProto(nil, true); got != nil {
		t.Errorf("SnapshotToProto(nil, true) = %v, want nil", got)
	}
}

func TestSnapshotToProto_WithoutIDHash(t *testing.T) {
	prevIDHash := Hash{0x01, 0x02}
	fileHash := Hash{0x22}
	chunkHash := Hash{0x11}
	prevID := &SnapshotID{Hash: prevIDHash}
	s := &Snapshot{
		ID:        SnapshotID{Hash: Hash{0xab, 0xcd}},
		PrevID:    prevID,
		Message:   "msg",
		Author:    "author",
		Timestamp: 42,
		Tags:      []string{"v1", "v2"},
		TotalSize: 1024,
		Files: []FileEntry{
			{Path: "a.txt", Mode: FileModeRegular, Size: 100, ModTime: 1, Chunks: []Hash{chunkHash}, Hash: fileHash},
			{Path: "b.txt", Mode: FileModeRegular, Size: 200, ModTime: 2},
		},
	}

	p := SnapshotToProto(s, false)
	if p == nil {
		t.Fatal("expected non-nil proto")
	}
	if p.Message != "msg" {
		t.Errorf("Message: got %q, want %q", p.Message, "msg")
	}
	if p.Author != "author" {
		t.Errorf("Author: got %q, want %q", p.Author, "author")
	}
	if p.Timestamp != 42 {
		t.Errorf("Timestamp: got %d, want 42", p.Timestamp)
	}
	if p.TotalSize != 1024 {
		t.Errorf("TotalSize: got %d, want 1024", p.TotalSize)
	}
	if len(p.Tags) != 2 || p.Tags[0] != "v1" || p.Tags[1] != "v2" {
		t.Errorf("Tags: got %v, want [v1 v2]", p.Tags)
	}
	// Without ID hash, IdHash should be nil even though s.ID is set.
	if p.IdHash != nil {
		t.Errorf("IdHash should be nil with withIDHash=false, got %d bytes", len(p.IdHash))
	}
	// PrevID should be populated.
	if p.PrevIdHash == nil || !bytes.Equal(p.PrevIdHash, prevIDHash[:]) {
		t.Errorf("PrevIdHash: got %v, want %v", p.PrevIdHash, prevIDHash[:])
	}
	if len(p.Files) != 2 {
		t.Fatalf("Files: got %d, want 2", len(p.Files))
	}
	if p.Files[0].Path != "a.txt" {
		t.Errorf("Files[0].Path: got %q, want %q", p.Files[0].Path, "a.txt")
	}
	if len(p.Files[0].ChunkHashes) != 1 {
		t.Errorf("Files[0].ChunkHashes: got %d, want 1", len(p.Files[0].ChunkHashes))
	}
	if p.Files[0].FileHash == nil || !bytes.Equal(p.Files[0].FileHash, fileHash[:]) {
		t.Errorf("Files[0].FileHash: got %v, want %v", p.Files[0].FileHash, fileHash[:])
	}
}

func TestSnapshotToProto_WithIDHash(t *testing.T) {
	idHash := Hash{0xab, 0xcd}
	s := &Snapshot{
		ID:      SnapshotID{Hash: idHash},
		Message: "msg",
	}
	p := SnapshotToProto(s, true)
	if p == nil {
		t.Fatal("expected non-nil proto")
	}
	if p.IdHash == nil || !bytes.Equal(p.IdHash, idHash[:]) {
		t.Errorf("IdHash: got %v, want %v", p.IdHash, idHash[:])
	}
}

func TestSnapshotToProto_WithIDHashButZeroID(t *testing.T) {
	s := &Snapshot{Message: "msg"}
	p := SnapshotToProto(s, true)
	if p == nil {
		t.Fatal("expected non-nil proto")
	}
	// Zero ID should not populate IdHash even when withIDHash is true.
	if p.IdHash != nil {
		t.Errorf("IdHash should be nil for zero ID, got %d bytes", len(p.IdHash))
	}
}

func TestSnapshotToProto_NoPrevID(t *testing.T) {
	s := &Snapshot{Message: "first"}
	p := SnapshotToProto(s, false)
	if p.PrevIdHash != nil {
		t.Errorf("PrevIdHash should be nil for first snapshot, got %d bytes", len(p.PrevIdHash))
	}
}

func TestSnapshotToProto_Metadata(t *testing.T) {
	s := &Snapshot{
		Files: []FileEntry{
			{
				Path:     "img.png",
				Metadata: &FileMetadata{MIMEType: "image/png", Extra: map[string]string{"w": "100", "h": "200"}},
			},
			{
				Path: "empty.txt",
				// nil Metadata
			},
			{
				Path:     "nomime.txt",
				Metadata: &FileMetadata{Extra: map[string]string{"k": "v"}},
			},
		},
	}
	p := SnapshotToProto(s, false)
	if p.Files[0].MimeType == nil || *p.Files[0].MimeType != "image/png" {
		t.Errorf("Files[0].MimeType: got %v, want %q", p.Files[0].MimeType, "image/png")
	}
	if len(p.Files[0].Extra) != 2 || p.Files[0].Extra["w"] != "100" || p.Files[0].Extra["h"] != "200" {
		t.Errorf("Files[0].Extra: got %v", p.Files[0].Extra)
	}
	if p.Files[1].MimeType != nil {
		t.Errorf("Files[1].MimeType should be nil, got %v", p.Files[1].MimeType)
	}
	if p.Files[1].Extra != nil {
		t.Errorf("Files[1].Extra should be nil, got %v", p.Files[1].Extra)
	}
	if p.Files[2].MimeType != nil {
		t.Errorf("Files[2].MimeType should be nil for empty MIMEType, got %v", p.Files[2].MimeType)
	}
	if len(p.Files[2].Extra) != 1 || p.Files[2].Extra["k"] != "v" {
		t.Errorf("Files[2].Extra: got %v", p.Files[2].Extra)
	}
}

func TestSnapshotToProto_DefensiveCopy(t *testing.T) {
	originalChunks := []Hash{{0x11}}
	s := &Snapshot{
		Files: []FileEntry{
			{Path: "a.txt", Chunks: originalChunks, Hash: Hash{0x22}},
		},
	}
	p := SnapshotToProto(s, false)

	// Mutate the original snapshot's chunk slice; the proto's chunk hashes
	// should be unaffected because fileEntryToProto copies each hash slice.
	s.Files[0].Chunks[0] = Hash{0xff}

	got := Hash{}
	copy(got[:], p.Files[0].ChunkHashes[0])
	if got == (Hash{0xff}) {
		t.Errorf("ChunkHashes should be defensively copied")
	}
	if got != (Hash{0x11}) {
		t.Errorf("ChunkHashes[0]: got %v, want %v", got, Hash{0x11})
	}
}

func TestSnapshotToProto_EmptyChunks(t *testing.T) {
	s := &Snapshot{
		Files: []FileEntry{{Path: "empty.txt"}},
	}
	p := SnapshotToProto(s, false)
	if p.Files[0].ChunkHashes != nil {
		t.Errorf("ChunkHashes should be nil for empty Chunks, got %d entries", len(p.Files[0].ChunkHashes))
	}
}
