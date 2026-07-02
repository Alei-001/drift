package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
)

// TestComputeRestoreChanges_HashDetectsSameSizeChange verifies that a file
// whose content changed but whose size and modtime were preserved (as with
// "cp -p") is correctly flagged as modified via content-hash comparison.
func TestComputeRestoreChanges_HashDetectsSameSizeChange(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")

	// Original content.
	orig := []byte("AAAAA")
	if err := os.WriteFile(filePath, orig, 0644); err != nil {
		t.Fatalf("write orig: %v", err)
	}

	// Capture modtime to restore later (simulates "cp -p").
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat orig: %v", err)
	}
	origModTime := info.ModTime()

	cfg := core.DefaultConfig().Core
	cfg.IgnoreFile = ""

	// Compute the hash the way CreateSnapshot would.
	origHash, err := porcelain.ComputeFileHash(filePath, &cfg)
	if err != nil {
		t.Fatalf("ComputeFileHash orig: %v", err)
	}

	// Build a snapshot entry for the original file.
	snap := &core.Snapshot{
		Files: []core.FileEntry{
			{Path: "file.txt", Size: int64(len(orig)), Hash: origHash, ModTime: origModTime.UnixNano()},
		},
	}

	// Overwrite with different content of the same size, then restore the
	// original modtime — exactly what "cp -p" does.
	if err := os.WriteFile(filePath, []byte("BBBBB"), 0644); err != nil {
		t.Fatalf("write modified: %v", err)
	}
	if err := os.Chtimes(filePath, origModTime, origModTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	added, modified, deleted, err := computeRestoreChanges(dir, &cfg, snap)
	if err != nil {
		t.Fatalf("computeRestoreChanges: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("expected 0 added, got %d: %v", len(added), added)
	}
	if len(deleted) != 0 {
		t.Errorf("expected 0 deleted, got %d: %v", len(deleted), deleted)
	}
	if len(modified) != 1 || modified[0].Path != "file.txt" {
		t.Errorf("expected file.txt in modified, got %d: %v", len(modified), modified)
	}
}

// TestComputeRestoreChanges_IdenticalFileNotModified verifies that an
// unchanged file is not flagged as modified.
func TestComputeRestoreChanges_IdenticalFileNotModified(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")

	orig := []byte("AAAAA")
	if err := os.WriteFile(filePath, orig, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := core.DefaultConfig().Core
	cfg.IgnoreFile = ""

	origHash, err := porcelain.ComputeFileHash(filePath, &cfg)
	if err != nil {
		t.Fatalf("ComputeFileHash: %v", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	snap := &core.Snapshot{
		Files: []core.FileEntry{
			{Path: "file.txt", Size: int64(len(orig)), Hash: origHash, ModTime: info.ModTime().UnixNano()},
		},
	}

	added, modified, deleted, err := computeRestoreChanges(dir, &cfg, snap)
	if err != nil {
		t.Fatalf("computeRestoreChanges: %v", err)
	}
	if len(added) != 0 || len(modified) != 0 || len(deleted) != 0 {
		t.Errorf("expected no changes, got added=%d modified=%d deleted=%d",
			len(added), len(modified), len(deleted))
	}
}

// TestComputeRestoreChanges_DifferentSizeModified verifies that a size
// difference is detected without needing a hash comparison.
func TestComputeRestoreChanges_DifferentSizeModified(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")

	if err := os.WriteFile(filePath, []byte("AAAAA"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := core.DefaultConfig().Core
	cfg.IgnoreFile = ""

	origHash, err := porcelain.ComputeFileHash(filePath, &cfg)
	if err != nil {
		t.Fatalf("ComputeFileHash: %v", err)
	}

	snap := &core.Snapshot{
		Files: []core.FileEntry{
			{Path: "file.txt", Size: 5, Hash: origHash},
		},
	}

	// Different size.
	if err := os.WriteFile(filePath, []byte("AAAAAAAAAA"), 0644); err != nil {
		t.Fatalf("write modified: %v", err)
	}

	_, modified, _, err := computeRestoreChanges(dir, &cfg, snap)
	if err != nil {
		t.Fatalf("computeRestoreChanges: %v", err)
	}
	if len(modified) != 1 || modified[0].Path != "file.txt" {
		t.Errorf("expected file.txt modified, got %d: %v", len(modified), modified)
	}
}
