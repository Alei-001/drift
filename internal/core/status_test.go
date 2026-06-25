package core

import "testing"

// TestStatus_File_LazyCreate verifies that File creates a new entry with default Unmodified status.
func TestStatus_File_LazyCreate(t *testing.T) {
	s := Status{}
	fs := s.File("a.txt")
	if fs == nil {
		t.Fatal("expected non-nil FileStatus")
	}
	if fs.Staging != Unmodified || fs.Worktree != Unmodified {
		t.Fatalf("expected Unmodified/Unmodified, got %q/%q", fs.Staging, fs.Worktree)
	}
	if _, ok := s["a.txt"]; !ok {
		t.Fatal("expected a.txt to be stored in the Status map")
	}
}

// TestStatus_File_Existing verifies that File returns the same pointer on subsequent calls.
func TestStatus_File_Existing(t *testing.T) {
	s := Status{}
	first := s.File("a.txt")
	first.Staging = Modified
	second := s.File("a.txt")
	if second.Staging != Modified {
		t.Fatalf("expected second call to return updated status, got %q", second.Staging)
	}
}

// TestStatus_IsClean_Empty verifies that an empty status is clean.
func TestStatus_IsClean_Empty(t *testing.T) {
	s := Status{}
	if !s.IsClean() {
		t.Fatal("empty status should be clean")
	}
}

// TestStatus_IsClean_AllUnmodified verifies that all-unmodified statuses are clean.
func TestStatus_IsClean_AllUnmodified(t *testing.T) {
	s := Status{}
	s.File("a.txt")
	s.File("b.txt")
	if !s.IsClean() {
		t.Fatal("all-unmodified status should be clean")
	}
}

// TestStatus_IsClean_WithChanges verifies that any non-unmodified entry makes status not clean.
func TestStatus_IsClean_WithChanges(t *testing.T) {
	s := Status{}
	s.File("a.txt")
	s.File("b.txt").Staging = Modified
	if s.IsClean() {
		t.Fatal("status with a Modified entry should not be clean")
	}
}

// TestStatusCode_String verifies the string representation of each StatusCode.
func TestStatusCode_String(t *testing.T) {
	cases := []struct {
		code StatusCode
		want string
	}{
		{Unmodified, " "},
		{Untracked, "?"},
		{Modified, "M"},
		{Added, "A"},
		{Deleted, "D"},
	}
	for _, c := range cases {
		if got := c.code.String(); got != c.want {
			t.Fatalf("StatusCode(%q).String() = %q, want %q", c.code, got, c.want)
		}
	}
}
