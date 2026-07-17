package snapshot

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/memory"
)

// exportTestSnapshot builds a snapshot with two file entries referencing
// stored chunks, so ExportSnapshot can reconstruct them into a zip. Returns
// the snapshot ID and the store.
func exportTestSnapshot(t *testing.T, st *store.StoreSet, tag byte) core.SnapshotID {
	t.Helper()
	c1 := gcChunk([]byte("file-alpha-content"), 18)
	c2 := gcChunk([]byte("file-beta-content"), 17)

	// Compute the file-level hash so integrity's file-hash check passes.
	fileAHash := computeFileHashFromHashes([]core.Hash{c1.Hash})
	fileBHash := computeFileHashFromHashes([]core.Hash{c2.Hash})

	files := []core.FileEntry{
		{
			Path:   "alpha.txt",
			Mode:   core.FileMode(0o644),
			Size:   int64(len(c1.Data)),
			Chunks: []core.Hash{c1.Hash},
			Hash:   fileAHash,
		},
		{
			Path:   "subdir/beta.txt",
			Mode:   core.FileMode(0o644),
			Size:   int64(len(c2.Data)),
			Chunks: []core.Hash{c2.Hash},
			Hash:   fileBHash,
		},
	}
	st.Chunks.PutChunk(context.Background(), c1)
	st.Chunks.PutChunk(context.Background(), c2)
	snap := &core.Snapshot{
		Timestamp: int64(tag),
		PrevID:    nil,
		Files:     files,
	}
	snap.ID = computeSnapshotID(snap)
	st.Snapshots.PutSnapshot(context.Background(), snap)
	return snap.ID
}

// TestExportSnapshot_NormalExport writes a snapshot's files to a zip archive
// and verifies the archive contains the expected files with correct content.
func TestExportSnapshot_NormalExport(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	snapID := exportTestSnapshot(t, st, 0x01)

	outPath := filepath.Join(t.TempDir(), "export.zip")
	result, err := ExportSnapshot(context.Background(), st, snapID, outPath)
	if err != nil {
		t.Fatalf("ExportSnapshot failed: %v", err)
	}
	if result.FileCount != 2 {
		t.Errorf("expected 2 files exported, got %d", result.FileCount)
	}

	// Open the zip and verify contents.
	r, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer r.Close()

	contents := make(map[string]string)
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		contents[f.Name] = string(data)
	}

	if contents["alpha.txt"] != "file-alpha-content" {
		t.Errorf("alpha.txt content = %q, want %q", contents["alpha.txt"], "file-alpha-content")
	}
	if contents["subdir/beta.txt"] != "file-beta-content" {
		t.Errorf("subdir/beta.txt content = %q, want %q", contents["subdir/beta.txt"], "file-beta-content")
	}
}

// TestExportSnapshot_NonExistentSnapshot verifies that exporting a snapshot
// ID that does not exist returns an error wrapping the storage error.
func TestExportSnapshot_NonExistentSnapshot(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	missingID := core.SnapshotID{Hash: gcHash(0xff)}

	outPath := filepath.Join(t.TempDir(), "export.zip")
	_, err := ExportSnapshot(context.Background(), st, missingID, outPath)
	if err == nil {
		t.Fatal("expected error for non-existent snapshot, got nil")
	}
	if !errors.Is(err, store.ErrNotFound) {
		// ExportSnapshot wraps the GetSnapshot error with "load snapshot:".
		// The underlying error should be ErrNotFound, but because it is
		// wrapped with fmt.Errorf("load snapshot: %w", err), errors.Is
		// should still find it.
		t.Errorf("expected error to wrap store.ErrNotFound, got %v", err)
	}
}

// TestExportSnapshot_OutputPathCreated verifies that the output directory is
// created when it does not exist.
func TestExportSnapshot_OutputPathCreated(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	snapID := exportTestSnapshot(t, st, 0x02)

	// Output path in a non-existent subdirectory.
	outDir := filepath.Join(t.TempDir(), "nested", "deep")
	outPath := filepath.Join(outDir, "export.zip")

	_, err := ExportSnapshot(context.Background(), st, snapID, outPath)
	if err != nil {
		t.Fatalf("ExportSnapshot failed: %v", err)
	}

	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("expected output file to exist, got: %v", err)
	}
}

// TestExportSnapshot_OverwriteExisting verifies that exporting to a path
// that already exists overwrites the file.
func TestExportSnapshot_OverwriteExisting(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	snapID := exportTestSnapshot(t, st, 0x03)

	outPath := filepath.Join(t.TempDir(), "export.zip")
	// Write a pre-existing file.
	if err := os.WriteFile(outPath, []byte("old content"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	_, err := ExportSnapshot(context.Background(), st, snapID, outPath)
	if err != nil {
		t.Fatalf("ExportSnapshot failed: %v", err)
	}

	// The file should be replaced with the zip.
	r, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("expected zip file after overwrite, got: %v", err)
	}
	r.Close()
}

// TestExportSnapshot_EmptySnapshot verifies that exporting a snapshot with
// no files produces a valid (empty) zip archive with FileCount=0.
func TestExportSnapshot_EmptySnapshot(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	snapHash := gcPutSnapshot(st, 0x04, nil, nil)

	outPath := filepath.Join(t.TempDir(), "empty.zip")
	result, err := ExportSnapshot(context.Background(), st, core.SnapshotID{Hash: snapHash}, outPath)
	if err != nil {
		t.Fatalf("ExportSnapshot failed: %v", err)
	}
	if result.FileCount != 0 {
		t.Errorf("expected 0 files, got %d", result.FileCount)
	}

	r, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer r.Close()
	if len(r.File) != 0 {
		t.Errorf("expected 0 entries in zip, got %d", len(r.File))
	}
}

// TestExportSnapshot_UnwritableOutputPath verifies that exporting to an
// unwritable path returns an error.
func TestExportSnapshot_UnwritableOutputPath(t *testing.T) {
	store := store.NewStoreSet(memory.NewMemoryStorage())
	snapID := exportTestSnapshot(t, st, 0x05)

	var outPath string
	if runtime.GOOS == "windows" {
		// On Windows, a path containing an invalid character like '<' will
		// cause os.MkdirAll/os.Create to fail.
		outPath = filepath.Join(t.TempDir(), "bad<name", "export.zip")
	} else {
		// On Unix, use a path under a regular file: MkdirAll fails with
		// ENOTDIR because a parent path component is a file, not a directory.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
			t.Fatalf("create blocker: %v", err)
		}
		outPath = filepath.Join(blocker, "export.zip")
	}
	_, err := ExportSnapshot(context.Background(), st, snapID, outPath)
	if err == nil {
		t.Fatal("expected error for unwritable path, got nil")
	}
}
