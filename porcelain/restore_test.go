package porcelain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/drift/core"
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

	snap1, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
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
	if _, err := RestoreSnapshot(context.Background(), store, dir, snap1.ID, "", true, nil); err != nil {
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
	headRef, err := store.GetRef(context.Background(), "HEAD")
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

	snap1, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot failed: %v", err)
	}

	// Modify file2 and create second snapshot.
	// Use different-length content so CreateSnapshot detects the change
	// (it short-circuits on matching size+modtime).
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2-v2-modified"), 0644); err != nil {
		t.Fatalf("modify file2: %v", err)
	}

	snap2, err := CreateSnapshot(context.Background(), store, dir, "second commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("second CreateSnapshot failed: %v", err)
	}

	// Single-file restore file2.txt to first snapshot (noBackup=true)
	if _, err := RestoreSnapshot(context.Background(), store, dir, snap1.ID, "file2.txt", true, nil); err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	// Verify HEAD still points to snap2 (single-file restore must not move HEAD)
	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.Target != snap2.ID.Hash {
		t.Errorf("HEAD should still point to snap2, got %s, expected %s",
			headRef.Target.String(), snap2.ID.Hash.String())
	}

	// Verify index contains all file entries (file1, file2, file3, .driftignore)
	idx, err := store.GetIndex(context.Background())
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

// TestRestore_RejectsSymlinkTraversal verifies that restore refuses to
// write through a symlink inside the workspace that points outside it.
// The snapshot contains a real "evil/x" entry; the workspace then replaces
// the real evil/ directory with a symlink to an external temp dir. Restore
// must reject writing evil/x and must not create files (or .drifttmp
// temporaries) inside the symlink target.
func TestRestore_RejectsSymlinkTraversal(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	store, _, err := OpenProject(dir)
	if err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Snapshot a real file at evil/x so the snapshot has a legit
	// FileEntry for that path with stored chunks.
	if err := os.MkdirAll(filepath.Join(dir, "evil"), 0755); err != nil {
		t.Fatalf("mkdir evil: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evil", "x"), []byte("evil-x-content"), 0644); err != nil {
		t.Fatalf("write evil/x: %v", err)
	}
	snap, err := CreateSnapshot(ctx, store, dir, "with evil/x", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// External directory the malicious symlink will point at.
	outsideDir := t.TempDir()
	outsideTarget := filepath.Join(outsideDir, "x")
	outsideTmp := filepath.Join(outsideDir, "x.drifttmp")

	// Replace the real evil/ directory with a symlink escaping the
	// workspace. Skip the test on systems that cannot create symlinks
	// (e.g. Windows without developer mode / admin privileges).
	if err := os.RemoveAll(filepath.Join(dir, "evil")); err != nil {
		t.Fatalf("remove evil dir: %v", err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(dir, "evil")); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}

	t.Run("single_file_restore", func(t *testing.T) {
		// Single-file restore of evil/x must fail and must not create
		// files (or .drifttmp temporaries) in the symlink target.
		if _, err := RestoreSnapshot(ctx, store, dir, snap.ID, "evil/x", true, nil); err == nil {
			t.Fatal("expected restore of evil/x to fail, got nil error")
		}
		if _, statErr := os.Stat(outsideTarget); !os.IsNotExist(statErr) {
			t.Errorf("expected outside target %s to not exist, got err=%v", outsideTarget, statErr)
		}
		if _, statErr := os.Stat(outsideTmp); !os.IsNotExist(statErr) {
			t.Errorf("expected outside tmp %s to not exist, got err=%v", outsideTmp, statErr)
		}
	})

	t.Run("full_restore", func(t *testing.T) {
		// Full restore must record a failure for evil/x and must not
		// create files (or .drifttmp temporaries) in the symlink target.
		if _, err := RestoreSnapshot(ctx, store, dir, snap.ID, "", true, nil); err == nil {
			t.Fatal("expected full restore to fail due to evil/x, got nil error")
		}
		if _, statErr := os.Stat(outsideTarget); !os.IsNotExist(statErr) {
			t.Errorf("expected outside target %s to not exist, got err=%v", outsideTarget, statErr)
		}
		if _, statErr := os.Stat(outsideTmp); !os.IsNotExist(statErr) {
			t.Errorf("expected outside tmp %s to not exist, got err=%v", outsideTmp, statErr)
		}
	})
}

// TestRestore_PartialFailureSkipsCleanup verifies that when some files
// fail to restore, the cleanup phase (deleting non-snapshot files) is
// skipped so the workspace is not left in an inconsistent state.
func TestRestore_PartialFailureSkipsCleanup(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	store, _, err := OpenProject(dir)
	if err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create two files and snapshot them.
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	snap, err := CreateSnapshot(ctx, store, dir, "initial", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// Find file2.txt's chunk hashes in the stored snapshot so we can
	// delete them and force a restore failure for that file.
	storedSnap, err := store.GetSnapshot(ctx, snap.ID)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	var file2Chunks []core.Hash
	for _, entry := range storedSnap.Files {
		if entry.Path == "file2.txt" {
			file2Chunks = entry.Chunks
			break
		}
	}
	if len(file2Chunks) == 0 {
		t.Fatalf("no chunks found for file2.txt in snapshot")
	}

	// Delete file2.txt's chunks to simulate a restore failure.
	for _, h := range file2Chunks {
		if err := store.DeleteChunk(ctx, h); err != nil {
			t.Fatalf("DeleteChunk %s failed: %v", h.String(), err)
		}
	}

	// Modify file1.txt so we can verify it was actually restored.
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("modified"), 0644); err != nil {
		t.Fatalf("modify file1: %v", err)
	}

	// Add an extra file not in the snapshot. This file must survive the
	// partial-failure restore (cleanup must be skipped).
	extraPath := filepath.Join(dir, "extra.txt")
	if err := os.WriteFile(extraPath, []byte("extra content"), 0644); err != nil {
		t.Fatalf("write extra file: %v", err)
	}

	// Restore with noBackup=true to isolate the restore failure.
	_, err = RestoreSnapshot(ctx, store, dir, snap.ID, "", true, nil)
	if err == nil {
		t.Fatal("expected restore to fail due to missing chunk, got nil error")
	}

	// Verify extra file was NOT deleted (cleanup skipped on partial failure).
	if _, statErr := os.Stat(extraPath); statErr != nil {
		t.Errorf("extra file should still exist after partial restore failure, got err=%v", statErr)
	}

	// Verify file1.txt was restored to the snapshot content.
	content, err := os.ReadFile(filepath.Join(dir, "file1.txt"))
	if err != nil {
		t.Fatalf("file1.txt should exist: %v", err)
	}
	if string(content) != "content1" {
		t.Errorf("expected file1.txt content 'content1', got %q", string(content))
	}
}

// TestRestore_PartialFailureReturnsBackupID verifies that when a restore
// fails after a backup snapshot was created, the returned error includes
// the backup snapshot ID so the user can roll back.
func TestRestore_PartialFailureReturnsBackupID(t *testing.T) {
	dir := t.TempDir()

	if err := InitProject(dir); err != nil {
		t.Fatalf("InitProject failed: %v", err)
	}

	store, _, err := OpenProject(dir)
	if err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create two files and snapshot them.
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	snap, err := CreateSnapshot(ctx, store, dir, "initial", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// Find file2.txt's chunk hashes in the stored snapshot.
	storedSnap, err := store.GetSnapshot(ctx, snap.ID)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	var file2Chunks []core.Hash
	for _, entry := range storedSnap.Files {
		if entry.Path == "file2.txt" {
			file2Chunks = entry.Chunks
			break
		}
	}
	if len(file2Chunks) == 0 {
		t.Fatalf("no chunks found for file2.txt in snapshot")
	}

	// Delete file2.txt's chunks to simulate a restore failure.
	for _, h := range file2Chunks {
		if err := store.DeleteChunk(ctx, h); err != nil {
			t.Fatalf("DeleteChunk %s failed: %v", h.String(), err)
		}
	}

	// Modify file1.txt so the workspace has changes. This ensures a
	// backup snapshot is created (rather than ErrNothingToSave).
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("modified"), 0644); err != nil {
		t.Fatalf("modify file1: %v", err)
	}

	// Restore with noBackup=false so a backup snapshot is created before
	// the restore attempt.
	backupID, err := RestoreSnapshot(ctx, store, dir, snap.ID, "", false, nil)
	if err == nil {
		t.Fatal("expected restore to fail due to missing chunk, got nil error")
	}

	// Verify a backup snapshot was created and returned.
	if backupID == "" {
		t.Fatal("expected non-empty backup ID after partial restore failure with backup enabled")
	}

	// Verify the error message contains the backup snapshot ID so the
	// user can use it to roll back.
	if !strings.Contains(err.Error(), backupID) {
		t.Errorf("error message should contain backup ID %q, got: %v", backupID, err)
	}
}
