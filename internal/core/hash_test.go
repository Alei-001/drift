package core

import (
	"strings"
	"testing"
)

func TestHash_String(t *testing.T) {
	h := Hash{0xab, 0xcd, 0xef}
	got := h.String()
	// String() returns the first 8 hex characters of the hash.
	if len(got) != 8 {
		t.Errorf("expected 8-char string, got %d chars: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "abcdef") {
		t.Errorf("expected string to start with 'abcdef', got %q", got)
	}
}

func TestHash_FullString(t *testing.T) {
	h := Hash{0xab, 0xcd, 0xef}
	got := h.FullString()
	// FullString() returns the full 64-character hex representation.
	if len(got) != 64 {
		t.Errorf("expected 64-char string, got %d chars: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "abcdef") {
		t.Errorf("expected string to start with 'abcdef', got %q", got)
	}
}

func TestHash_IsZero(t *testing.T) {
	if !(Hash{}).IsZero() {
		t.Error("empty hash should be zero")
	}
	if (Hash{0x01}).IsZero() {
		t.Error("hash with byte should not be zero")
	}
}

func TestHash_Equality(t *testing.T) {
	a := Hash{0x01, 0x02}
	b := Hash{0x01, 0x02}
	c := Hash{0x01, 0x03}

	if a != b {
		t.Error("identical hashes should be equal")
	}
	if a == c {
		t.Error("different hashes should not be equal")
	}
}

func TestSnapshotID_HashIsZero(t *testing.T) {
	// SnapshotID does not expose its own IsZero method; zero-ness is
	// checked via the embedded Hash field.
	if !(SnapshotID{}).Hash.IsZero() {
		t.Error("empty SnapshotID's Hash should be zero")
	}
	if (SnapshotID{Hash: Hash{0x01}}).Hash.IsZero() {
		t.Error("SnapshotID with non-zero Hash should not be zero")
	}
}

func TestSnapshot_ShortID(t *testing.T) {
	s := &Snapshot{ID: SnapshotID{Hash: Hash{0xab, 0xcd, 0xef}}}
	got := s.ShortID()
	if len(got) != 8 {
		t.Errorf("expected 8-char ShortID, got %d chars: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "abcdef") {
		t.Errorf("expected ShortID to start with 'abcdef', got %q", got)
	}
}

func TestSnapshot_FullID(t *testing.T) {
	s := &Snapshot{ID: SnapshotID{Hash: Hash{0xab, 0xcd, 0xef}}}
	got := s.FullID()
	if len(got) != 64 {
		t.Errorf("expected 64-char FullID, got %d chars: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "abcdef") {
		t.Errorf("expected FullID to start with 'abcdef', got %q", got)
	}
}
