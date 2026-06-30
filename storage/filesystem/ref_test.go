package filesystem

import (
	"context"
	"testing"

	"github.com/your-org/drift/core"
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
