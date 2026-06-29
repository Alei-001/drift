package porcelain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRestore_FullRestoreDeletesExtraFiles(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	store, _, err := OpenProject(dir)
	if err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}
	defer store.Close()

	// Create initial file and snapshot
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v1"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	snap1, err := CreateSnapshot(store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// Add an extra file not in the snapshot
	extraPath := filepath.Join(dir, "extra.txt")
	if err := os.WriteFile(extraPath, []byte("extra content"), 0644); err != nil {
		t.Fatalf("write extra file: %v", err)
	}
	if _, err := os.Stat(extraPath); err != nil {
		t.Fatalf("extra file should exist before restore: %v", err)
	}

	// Full restore to first snapshot (noBackup=true to skip backup)
	if _, err := RestoreSnapshot(store, dir, snap1.ID, "", true); err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	// Verify extra file was deleted
	if _, err := os.Stat(extraPath); !os.IsNotExist(err) {
		t.Errorf("expected extra file to be deleted after full restore, got err=%v", err)
	}

	// Verify file1.txt still exists with original content
	content, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatalf("file1.txt should exist after restore: %v", err)
	}
	if string(content) != "content v1" {
		t.Errorf("expected 'content v1', got %q", string(content))
	}

	// Verify HEAD points to snap1
	headRef, err := store.GetRef("HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.Target != snap1.ID.Hash {
		t.Errorf("HEAD should point to snap1 after full restore")
	}
}

func TestRestore_SingleFilePreservesIndex(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	store, _, err := OpenProject(dir)
	if err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}
	defer store.Close()

	// Create multiple files and first snapshot
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2-v1"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file3.txt"), []byte("content3"), 0644); err != nil {
		t.Fatalf("write file3: %v", err)
	}

	snap1, err := CreateSnapshot(store, dir, "first commit", "test", nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot failed: %v", err)
	}

	// Modify file2 and create second snapshot.
	// Use different-length content so CreateSnapshot detects the change
	// (it short-circuits on matching size+modtime).
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2-v2-modified"), 0644); err != nil {
		t.Fatalf("modify file2: %v", err)
	}

	snap2, err := CreateSnapshot(store, dir, "second commit", "test", nil)
	if err != nil {
		t.Fatalf("second CreateSnapshot failed: %v", err)
	}

	// Single-file restore file2.txt to first snapshot (noBackup=true)
	if _, err := RestoreSnapshot(store, dir, snap1.ID, "file2.txt", true); err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	// Verify HEAD still points to snap2 (single-file restore must not move HEAD)
	headRef, err := store.GetRef("HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.Target != snap2.ID.Hash {
		t.Errorf("HEAD should still point to snap2, got %s, expected %s",
			headRef.Target.String(), snap2.ID.Hash.String())
	}

	// Verify index contains all file entries (file1, file2, file3, .driftignore)
	idx, err := store.GetIndex()
	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}
	if len(idx.Entries) != 4 {
		t.Errorf("expected 4 index entries (3 files + .driftignore), got %d", len(idx.Entries))
	}

	// Verify file2.txt content was restored to v1
	content, err := os.ReadFile(filepath.Join(dir, "file2.txt"))
	if err != nil {
		t.Fatalf("file2.txt should exist: %v", err)
	}
	if string(content) != "content2-v1" {
		t.Errorf("expected 'content2-v1', got %q", string(content))
	}

	// Verify file1 and file3 are untouched
	if content, err := os.ReadFile(filepath.Join(dir, "file1.txt")); err != nil || string(content) != "content1" {
		t.Errorf("file1.txt should be unchanged, got %q, err=%v", string(content), err)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "file3.txt")); err != nil || string(content) != "content3" {
		t.Errorf("file3.txt should be unchanged, got %q, err=%v", string(content), err)
	}
}
