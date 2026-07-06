package porcelain

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage/backends/memory"
)

// TestResolveHeadSnapshot_NoHead verifies that a missing HEAD ref yields a
// nil snapshot rather than an error or panic.
func TestResolveHeadSnapshot_NoHead(t *testing.T) {
	// Use a fresh memory storage with no HEAD ref at all. The memory backend
	// refuses to DeleteRef("HEAD"), so we build the empty store by hand.
	store := memory.NewMemoryStorage()
	if snap := ResolveHeadSnapshot(context.Background(), store); snap != nil {
		t.Errorf("expected nil snapshot when HEAD missing, got %v", snap)
	}
}

// TestResolveHeadSnapshot_ZeroTarget verifies that a HEAD pointing at the
// zero hash (fresh project, no commits) yields nil.
func TestResolveHeadSnapshot_ZeroTarget(t *testing.T) {
	store := setupTestStore(t)
	// setupTestStore creates HEAD with SymRef but zero target.
	if snap := ResolveHeadSnapshot(context.Background(), store); snap != nil {
		t.Errorf("expected nil for zero target, got %v", snap)
	}
}

// TestResolveHeadSnapshot_MissingSnapshot verifies that a HEAD pointing at a
// snapshot that was never stored yields nil rather than panicking.
func TestResolveHeadSnapshot_MissingSnapshot(t *testing.T) {
	store := setupTestStore(t)
	snapHash := core.Hash{0x42}
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: snapHash,
	})
	// No snapshot stored under this hash.
	if snap := ResolveHeadSnapshot(context.Background(), store); snap != nil {
		t.Errorf("expected nil for missing snapshot, got %v", snap)
	}
}

// TestResolveHeadSnapshot_Success verifies the happy path: HEAD points at a
// stored snapshot, which is returned.
func TestResolveHeadSnapshot_Success(t *testing.T) {
	store := setupTestStore(t)
	snapHash := core.Hash{0x42}
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: snapHash,
	})
	want := &core.Snapshot{ID: core.SnapshotID{Hash: snapHash}, Message: "test"}
	store.PutSnapshot(context.Background(), want)

	got := ResolveHeadSnapshot(context.Background(), store)
	if got == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if got.ID.Hash != snapHash {
		t.Errorf("expected hash %x, got %x", snapHash, got.ID.Hash)
	}
	if got.Message != "test" {
		t.Errorf("expected message 'test', got %q", got.Message)
	}
}

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns whatever was
// printed. It restores os.Stdout even on failure. Used to test the
// print-bearing diff helpers in diff.go.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	return buf.String()
}

// TestDiffSnapshots_NoChanges verifies that two snapshots with identical file
// sets and chunk hashes report "No changes." in the output.
func TestDiffSnapshots_NoChanges(t *testing.T) {
	hashA := core.Hash{0xAA}
	snap1 := &core.Snapshot{
		ID:    core.SnapshotID{Hash: core.Hash{0x01}},
		Files: []core.FileEntry{{Path: "a", Hash: hashA, Size: 10, Chunks: []core.Hash{hashA}}},
	}
	snap2 := &core.Snapshot{
		ID:    core.SnapshotID{Hash: core.Hash{0x02}},
		Files: []core.FileEntry{{Path: "a", Hash: hashA, Size: 10, Chunks: []core.Hash{hashA}}},
	}
	out := captureStdout(t, func() { DiffSnapshots(nil, snap1, snap2) })
	if !strings.Contains(out, "No changes") {
		t.Errorf("expected 'No changes' in output, got %q", out)
	}
}

// TestDiffSnapshots_AddedModifiedDeleted verifies that DiffSnapshots classifies
// files into added, modified, and deleted buckets correctly.
func TestDiffSnapshots_AddedModifiedDeleted(t *testing.T) {
	hashA := core.Hash{0xAA}
	hashB := core.Hash{0xBB}
	hashB2 := core.Hash{0xBB, 0x01}
	hashC := core.Hash{0xCC}
	snap1 := &core.Snapshot{
		ID: core.SnapshotID{Hash: core.Hash{0x01}},
		Files: []core.FileEntry{
			{Path: "a", Hash: hashA, Size: 10, Chunks: []core.Hash{hashA}}, // unchanged
			{Path: "b", Hash: hashB, Size: 20, Chunks: []core.Hash{hashB}}, // modified
			{Path: "c", Hash: hashC, Size: 30, Chunks: []core.Hash{hashC}}, // deleted in snap2
		},
	}
	snap2 := &core.Snapshot{
		ID: core.SnapshotID{Hash: core.Hash{0x02}},
		Files: []core.FileEntry{
			{Path: "a", Hash: hashA, Size: 10, Chunks: []core.Hash{hashA}},  // unchanged
			{Path: "b", Hash: hashB2, Size: 25, Chunks: []core.Hash{hashB2}}, // modified (hash + size)
			{Path: "d", Hash: core.Hash{0xDD}, Size: 40, Chunks: []core.Hash{{0xDD}}}, // added
		},
	}
	out := captureStdout(t, func() { DiffSnapshots(nil, snap1, snap2) })
	if !strings.Contains(out, "+  d") {
		t.Errorf("expected '+  d' (added) in output, got %q", out)
	}
	if !strings.Contains(out, "~  b") {
		t.Errorf("expected '~  b' (modified) in output, got %q", out)
	}
	if !strings.Contains(out, "-  c") {
		t.Errorf("expected '-  c' (deleted) in output, got %q", out)
	}
	// 'a' is unchanged and should not appear in any +/-/~ line.
	if strings.Contains(out, "+  a") || strings.Contains(out, "~  a") || strings.Contains(out, "-  a") {
		t.Errorf("unchanged file 'a' should not appear in diff output: %q", out)
	}
}

// TestDiffSnapshots_EmptySnapshots verifies that diffing two empty snapshots
// reports "No changes." rather than panicking.
func TestDiffSnapshots_EmptySnapshots(t *testing.T) {
	snap1 := &core.Snapshot{ID: core.SnapshotID{Hash: core.Hash{0x01}}}
	snap2 := &core.Snapshot{ID: core.SnapshotID{Hash: core.Hash{0x02}}}
	out := captureStdout(t, func() { DiffSnapshots(nil, snap1, snap2) })
	if !strings.Contains(out, "No changes") {
		t.Errorf("expected 'No changes' for empty snapshots, got %q", out)
	}
}

// TestDiffWorkspaceVsSnapshot_NoChanges verifies that a workspace matching the
// snapshot content reports "No changes.".
func TestDiffWorkspaceVsSnapshot_NoChanges(t *testing.T) {
	store, dir := setupLockedProject(t)
	cfg := core.DefaultConfig().Core

	// Write a file, snapshot it, then diff workspace vs the snapshot.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	snap, err := CreateSnapshot(context.Background(), store, dir, "init", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	out := captureStdout(t, func() {
		_ = DiffWorkspaceVsSnapshot(store, dir, snap, &cfg)
	})
	if !strings.Contains(out, "No changes") {
		t.Errorf("expected 'No changes' for matching workspace, got %q", out)
	}
}

// TestDiffWorkspaceVsSnapshot_AddedModifiedDeleted verifies that the
// workspace-vs-snapshot diff detects added, modified, and deleted files.
func TestDiffWorkspaceVsSnapshot_AddedModifiedDeleted(t *testing.T) {
	store, dir := setupLockedProject(t)
	cfg := core.DefaultConfig().Core

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}
	snap, err := CreateSnapshot(context.Background(), store, dir, "init", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// Modify a.txt (size change), delete b.txt, add c.txt.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa-modified"), 0644); err != nil {
		t.Fatalf("modify a.txt: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatalf("remove b.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("ccc"), 0644); err != nil {
		t.Fatalf("write c.txt: %v", err)
	}

	out := captureStdout(t, func() {
		_ = DiffWorkspaceVsSnapshot(store, dir, snap, &cfg)
	})
	if !strings.Contains(out, "+  c.txt") {
		t.Errorf("expected '+  c.txt' (added), got %q", out)
	}
	if !strings.Contains(out, "~  a.txt") {
		t.Errorf("expected '~  a.txt' (modified), got %q", out)
	}
	if !strings.Contains(out, "-  b.txt") {
		t.Errorf("expected '-  b.txt' (deleted), got %q", out)
	}
}
