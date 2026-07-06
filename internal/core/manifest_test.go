package core

import (
	"bytes"
	"math/rand"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestManifest_RoundTrip(t *testing.T) {
	id := make([]byte, HashSize)
	rand.Read(id)
	prevID := make([]byte, HashSize)
	rand.Read(prevID)
	m := &SnapshotManifest{
		Id:           id,
		PrevId:       prevID,
		Message:      "test snapshot",
		Author:       "tester",
		Timestamp:    1700000000,
		Tags:         []string{"v1", "release"},
		TotalSize:    4096,
		FilesChanged: 42,
	}

	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal returned empty data")
	}

	var got SnapshotManifest
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !bytes.Equal(got.Id, m.Id) {
		t.Errorf("ID mismatch")
	}
	if !bytes.Equal(got.PrevId, m.PrevId) {
		t.Errorf("PrevID mismatch")
	}
	if got.Message != m.Message {
		t.Errorf("Message: got %q, want %q", got.Message, m.Message)
	}
	if got.Author != m.Author {
		t.Errorf("Author: got %q, want %q", got.Author, m.Author)
	}
	if got.Timestamp != m.Timestamp {
		t.Errorf("Timestamp: got %d, want %d", got.Timestamp, m.Timestamp)
	}
	if len(got.Tags) != len(m.Tags) {
		t.Errorf("Tags length: got %d, want %d", len(got.Tags), len(m.Tags))
	} else {
		for i, tag := range m.Tags {
			if got.Tags[i] != tag {
				t.Errorf("Tags[%d]: got %q, want %q", i, got.Tags[i], tag)
			}
		}
	}
	if got.TotalSize != m.TotalSize {
		t.Errorf("TotalSize: got %d, want %d", got.TotalSize, m.TotalSize)
	}
	if got.FilesChanged != m.FilesChanged {
		t.Errorf("FilesChanged: got %d, want %d", got.FilesChanged, m.FilesChanged)
	}
}

func TestManifest_NilPrevID_EmptyTags(t *testing.T) {
	id := make([]byte, HashSize)
	rand.Read(id)
	m := &SnapshotManifest{
		Id:        id,
		Message:   "first commit",
		Timestamp: 100,
	}

	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got SnapshotManifest
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.PrevId != nil {
		t.Errorf("PrevId: got %v, want nil", got.PrevId)
	}
	if got.Tags != nil {
		t.Errorf("Tags: got %v, want nil", got.Tags)
	}
}

func TestManifest_Unmarshal_EmptyData(t *testing.T) {
	var m SnapshotManifest
	if err := proto.Unmarshal([]byte{}, &m); err != nil {
		t.Fatalf("Unmarshal empty data failed: %v", err)
	}
	if m.Id != nil {
		t.Error("expected nil Id for empty data")
	}
}

func TestSnapshotToManifest_ManifestToSummary_RoundTrip(t *testing.T) {
	prevID := &SnapshotID{Hash: Hash{0x99}}
	s := &Snapshot{
		ID:        SnapshotID{Hash: Hash{0x01}},
		PrevID:    prevID,
		Message:   "msg",
		Author:    "author",
		Timestamp: 42,
		Tags:      []string{"tag1"},
		TotalSize: 100,
		Files:     []FileEntry{{Path: "a.txt"}, {Path: "b.txt"}},
	}

	m := SnapshotToManifest(s)
	if m == nil {
		t.Fatal("SnapshotToManifest returned nil")
	}
	if m.FilesChanged != 2 {
		t.Errorf("FilesChanged: got %d, want 2", m.FilesChanged)
	}

	got := ManifestToSummary(m)
	if got == nil {
		t.Fatal("ManifestToSummary returned nil")
	}
	if got.ID != s.ID {
		t.Errorf("ID mismatch")
	}
	if got.Message != s.Message {
		t.Errorf("Message mismatch")
	}
	if got.TotalSize != s.TotalSize {
		t.Errorf("TotalSize: got %d, want %d", got.TotalSize, s.TotalSize)
	}
	if got.PrevID == nil || got.PrevID.Hash != s.PrevID.Hash {
		t.Errorf("PrevID mismatch")
	}

	// SnapshotManifest with nil PrevId should produce a summary with nil PrevID.
	m2 := &SnapshotManifest{Id: s.ID.Hash[:], Message: "hello"}
	got2 := ManifestToSummary(m2)
	if got2.PrevID != nil {
		t.Errorf("expected nil PrevID for manifest without prev_id")
	}
}

func TestManifest_WireFormat_Compatible(t *testing.T) {
	id := Hash{0xde, 0xad}
	m := &SnapshotManifest{
		Id:           id[:],
		Message:      "hello",
		FilesChanged: 3,
	}

	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}

	var got SnapshotManifest
	if err := proto.Unmarshal(data, &got); err != nil {
		t.Fatalf("proto.Unmarshal: %v", err)
	}

	if !bytes.Equal(got.Id, id[:]) {
		t.Errorf("ID mismatch after round-trip")
	}
	if got.Message != "hello" {
		t.Errorf("message: got %q, want %q", got.Message, "hello")
	}
	if got.FilesChanged != 3 {
		t.Errorf("FilesChanged: got %d, want 3", got.FilesChanged)
	}
}
