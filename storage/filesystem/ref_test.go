package filesystem

import (
	"context"
	"errors"
	"testing"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
)

// TestRef_PathTraversal verifies that ref names containing path traversal
// sequences are rejected by GetRef, SetRef, and DeleteRef, preventing
// writes or reads outside the refs directory.
func TestRef_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	maliciousNames := []string{
		"../../etc/passwd",
		"..",
		"../foo",
		"foo/../bar",
	}

	for _, name := range maliciousNames {
		t.Run(name, func(t *testing.T) {
			// GetRef should fail.
			if _, err := fs.GetRef(context.Background(), name); err == nil {
				t.Errorf("GetRef(%q) should fail", name)
			}
			// SetRef should fail.
			if err := fs.SetRef(context.Background(), name, &core.Reference{Name: name, Target: core.Hash{}}); err == nil {
				t.Errorf("SetRef(%q) should fail", name)
			}
			// DeleteRef should fail.
			if err := fs.DeleteRef(context.Background(), name); err == nil {
				t.Errorf("DeleteRef(%q) should fail", name)
			}
		})
	}
}

// TestGetRef_SymRefSelfReference verifies that a HEAD file whose content is
// "ref: HEAD" (pointing at itself) does not cause unbounded recursion and
// instead returns an error wrapping storage.ErrInvalidRef.
func TestGetRef_SymRefSelfReference(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	// Write a HEAD that points to itself: "ref: HEAD".
	if err := fs.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "HEAD",
	}); err != nil {
		t.Fatalf("SetRef: %v", err)
	}

	_, err = fs.GetRef(context.Background(), "HEAD")
	if err == nil {
		t.Fatalf("GetRef(HEAD) should fail on self-referential symref")
	}
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("GetRef(HEAD) error should wrap storage.ErrInvalidRef, got: %v", err)
	}
}

// TestGetRef_SymRefCycle verifies that a cycle of symbolic references
// (A -> B -> A) is detected and reported as storage.ErrInvalidRef rather
// than recursing forever.
func TestGetRef_SymRefCycle(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	// Create a cycle: heads/A -> heads/B -> heads/A.
	if err := fs.SetRef(context.Background(), "heads/A", &core.Reference{
		Name:   "heads/A",
		Type:   core.RefTypeBranch,
		SymRef: "heads/B",
	}); err != nil {
		t.Fatalf("SetRef heads/A: %v", err)
	}
	if err := fs.SetRef(context.Background(), "heads/B", &core.Reference{
		Name:   "heads/B",
		Type:   core.RefTypeBranch,
		SymRef: "heads/A",
	}); err != nil {
		t.Fatalf("SetRef heads/B: %v", err)
	}

	_, err = fs.GetRef(context.Background(), "heads/A")
	if err == nil {
		t.Fatalf("GetRef(heads/A) should fail on symref cycle")
	}
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("GetRef(heads/A) error should wrap storage.ErrInvalidRef, got: %v", err)
	}
}

// TestGetRef_NonHeadSymRef verifies that symbolic references are resolved
// for refs other than HEAD: a branch pointing at another branch via "ref:"
// must resolve to the target's hash.
func TestGetRef_NonHeadSymRef(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	// heads/main points at a real hash.
	targetHash := core.Hash{0x01, 0x02, 0x03}
	if err := fs.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: targetHash,
	}); err != nil {
		t.Fatalf("SetRef heads/main: %v", err)
	}

	// heads/feature is a symref pointing at heads/main (not HEAD).
	if err := fs.SetRef(context.Background(), "heads/feature", &core.Reference{
		Name:   "heads/feature",
		Type:   core.RefTypeBranch,
		SymRef: "heads/main",
	}); err != nil {
		t.Fatalf("SetRef heads/feature: %v", err)
	}

	ref, err := fs.GetRef(context.Background(), "heads/feature")
	if err != nil {
		t.Fatalf("GetRef(heads/feature): %v", err)
	}
	if ref.SymRef != "heads/main" {
		t.Errorf("SymRef = %q, want %q", ref.SymRef, "heads/main")
	}
	if ref.Target != targetHash {
		t.Errorf("Target = %v, want %v", ref.Target, targetHash)
	}
}
