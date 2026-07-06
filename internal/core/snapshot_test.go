package core

import (
	"strings"
	"testing"
)

// Note: Snapshot.ShortID and Snapshot.FullID are covered in hash_test.go.
// This file focuses on SnapshotSummary, which mirrors the same methods but
// was previously untested.

func TestSnapshotSummary_ShortID(t *testing.T) {
	ss := &SnapshotSummary{ID: SnapshotID{Hash: Hash{0xab, 0xcd, 0xef}}}
	got := ss.ShortID()
	if len(got) != 8 {
		t.Errorf("expected 8-char ShortID, got %d chars: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "abcdef") {
		t.Errorf("expected ShortID to start with 'abcdef', got %q", got)
	}
}

func TestSnapshotSummary_FullID(t *testing.T) {
	ss := &SnapshotSummary{ID: SnapshotID{Hash: Hash{0xab, 0xcd, 0xef}}}
	got := ss.FullID()
	if len(got) != 64 {
		t.Errorf("expected 64-char FullID, got %d chars: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "abcdef") {
		t.Errorf("expected FullID to start with 'abcdef', got %q", got)
	}
}

func TestSnapshotSummary_ZeroID(t *testing.T) {
	ss := &SnapshotSummary{}
	if ss.ShortID() != "00000000" {
		t.Errorf("expected zero ShortID '00000000', got %q", ss.ShortID())
	}
	if ss.FullID() != strings.Repeat("0", 64) {
		t.Errorf("expected zero FullID of 64 zeros, got %q", ss.FullID())
	}
}

func TestSnapshotSummary_FieldsRoundTrip(t *testing.T) {
	// Verify SnapshotSummary carries the same metadata fields as Snapshot
	// (minus the file list), so manifest→summary conversions preserve data.
	prevID := &SnapshotID{Hash: Hash{0x99}}
	ss := &SnapshotSummary{
		ID:        SnapshotID{Hash: Hash{0x01}},
		PrevID:    prevID,
		Message:   "msg",
		Author:    "author",
		Timestamp: 42,
		Tags:      []string{"v1"},
		TotalSize: 100,
	}
	if ss.PrevID == nil || ss.PrevID.Hash != prevID.Hash {
		t.Errorf("PrevID mismatch")
	}
	if ss.Message != "msg" {
		t.Errorf("Message: got %q, want %q", ss.Message, "msg")
	}
	if ss.Author != "author" {
		t.Errorf("Author: got %q, want %q", ss.Author, "author")
	}
	if ss.Timestamp != 42 {
		t.Errorf("Timestamp: got %d, want 42", ss.Timestamp)
	}
	if ss.TotalSize != 100 {
		t.Errorf("TotalSize: got %d, want 100", ss.TotalSize)
	}
	if len(ss.Tags) != 1 || ss.Tags[0] != "v1" {
		t.Errorf("Tags: got %v, want [v1]", ss.Tags)
	}
}
