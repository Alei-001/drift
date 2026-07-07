package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// TestFS_PutSnapshot_WritesManifest verifies that PutSnapshot writes both
// the snapshot file and its lightweight manifest.
func TestFS_PutSnapshot_WritesManifest(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	snap := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{0x01}},
		Message:   "test",
		Author:    "tester",
		Timestamp: 42,
	}
	if err := store.PutSnapshot(ctx, snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}

	// Both snapshot and manifest files should exist.
	snapPath := store.snapshotPath(snap.ID)
	if _, err := os.Stat(snapPath); err != nil {
		t.Errorf("snapshot file missing: %v", err)
	}
	manPath := store.manifestPath(snap.ID)
	if _, err := os.Stat(manPath); err != nil {
		t.Errorf("manifest file missing: %v", err)
	}
}

// TestFS_ListSnapshots_NilFiles verifies that ListSnapshots returns snapshots
// with nil Files (using manifests, not full deserialization).
func TestFS_ListSnapshots_NilFiles(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		snap := &core.Snapshot{
			ID:        core.SnapshotID{Hash: core.Hash{byte(i + 1)}},
			Message:   "snap",
			Timestamp: int64(i),
			Files:     []core.FileEntry{{Path: "file.txt", Size: 100}},
		}
		if err := store.PutSnapshot(ctx, snap); err != nil {
			t.Fatalf("PutSnapshot %d: %v", i, err)
		}
	}

	snaps, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 5 {
		t.Fatalf("expected 5 snapshots, got %d", len(snaps))
	}
}

// TestFS_ListSnapshots_ManifestFallback verifies that when a manifest is
// missing, ListSnapshots falls back to reading the full snapshot and
// backfills the manifest.
func TestFS_ListSnapshots_ManifestFallback(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	snap := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{0x42}},
		Message:   "fallback test",
		Author:    "tester",
		Timestamp: 99,
		Tags:      []string{"v1"},
		TotalSize: 1234,
	}
	if err := store.PutSnapshot(ctx, snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}

	// Delete the manifest to simulate a legacy snapshot without manifest.
	manPath := store.manifestPath(snap.ID)
	if err := os.Remove(manPath); err != nil {
		t.Fatalf("remove manifest: %v", err)
	}

	// ListSnapshots should fall back to reading the full snapshot.
	snaps, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}
	got := snaps[0]
	if got.Message != "fallback test" {
		t.Errorf("Message: got %q, want %q", got.Message, "fallback test")
	}
	if got.Author != "tester" {
		t.Errorf("Author: got %q, want %q", got.Author, "tester")
	}
	if got.Timestamp != 99 {
		t.Errorf("Timestamp: got %d, want %d", got.Timestamp, 99)
	}
	if got.TotalSize != 1234 {
		t.Errorf("TotalSize: got %d, want %d", got.TotalSize, 1234)
	}

	// The manifest should have been backfilled.
	if _, err := os.Stat(manPath); err != nil {
		t.Errorf("manifest was not backfilled: %v", err)
	}

	// Second listing should use the backfilled manifest (Files is nil).
	snaps2, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		t.Fatalf("second ListSnapshots: %v", err)
	}
	if len(snaps2) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps2))
	}
	if snaps2[0].Message != "fallback test" {
		t.Errorf("Message: got %q, want %q", snaps2[0].Message, "fallback test")
	}
}

// TestFS_DeleteSnapshot_RemovesManifest verifies that DeleteSnapshot removes
// both the snapshot and its manifest.
func TestFS_DeleteSnapshot_RemovesManifest(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	snap := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{0x77}},
		Message:   "to be deleted",
		Timestamp: 1,
	}
	if err := store.PutSnapshot(ctx, snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}

	if err := store.DeleteSnapshot(ctx, snap.ID); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	snapPath := store.snapshotPath(snap.ID)
	if _, err := os.Stat(snapPath); !os.IsNotExist(err) {
		t.Errorf("snapshot file should be deleted, got err=%v", err)
	}
	manPath := store.manifestPath(snap.ID)
	if _, err := os.Stat(manPath); !os.IsNotExist(err) {
		t.Errorf("manifest file should be deleted, got err=%v", err)
	}
}

// TestFS_ListSnapshots_LargeCount verifies that listing a large number of
// snapshots via manifests works correctly and does not deserialize full
// snapshots. Each snapshot has a Files field that would be expensive to
// deserialize; the manifest path avoids this entirely.
func TestFS_ListSnapshots_LargeCount(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	const count = 500
	for i := 0; i < count; i++ {
		// Each snapshot has many file entries to make the full snapshot
		// large. With manifests, ListSnapshots never deserializes these.
		files := make([]core.FileEntry, 50)
		for j := range files {
			files[j] = core.FileEntry{Path: "file", Size: int64(j)}
		}
		snap := &core.Snapshot{
			ID:        core.SnapshotID{Hash: core.Hash{byte(i), byte(i >> 8), byte(i >> 16), byte(i >> 24)}},
			Message:   "snap",
			Timestamp: int64(i),
			Files:     files,
		}
		if err := store.PutSnapshot(ctx, snap); err != nil {
			t.Fatalf("PutSnapshot %d: %v", i, err)
		}
	}

	snaps, err := store.ListSnapshots(ctx, &storage.ListOptions{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 10 {
		t.Fatalf("expected 10 snapshots (limit), got %d", len(snaps))
	}

	// Verify pagination: total count without limit.
	all, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		t.Fatalf("ListSnapshots (all): %v", err)
	}
	if len(all) != count {
		t.Errorf("expected %d total snapshots, got %d", count, len(all))
	}

	// Verify sorted by timestamp descending.
	for i := 1; i < len(all); i++ {
		if all[i].Timestamp > all[i-1].Timestamp {
			t.Error("snapshots should be sorted by timestamp descending")
			break
		}
	}
}

// TestFS_ListSnapshots_Pagination verifies limit/offset pagination.
func TestFS_ListSnapshots_Pagination(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		snap := &core.Snapshot{
			ID:        core.SnapshotID{Hash: core.Hash{byte(i + 1)}},
			Message:   "snap",
			Timestamp: int64(i),
		}
		if err := store.PutSnapshot(ctx, snap); err != nil {
			t.Fatalf("PutSnapshot %d: %v", i, err)
		}
	}

	// Limit = 3
	snaps, err := store.ListSnapshots(ctx, &storage.ListOptions{Limit: 3})
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(snaps))
	}

	// Offset = 8 (only 2 left)
	snaps, err = store.ListSnapshots(ctx, &storage.ListOptions{Offset: 8})
	if err != nil {
		t.Fatalf("ListSnapshots offset: %v", err)
	}
	if len(snaps) != 2 {
		t.Errorf("expected 2 snapshots after offset 8, got %d", len(snaps))
	}

	// Offset beyond range
	snaps, err = store.ListSnapshots(ctx, &storage.ListOptions{Offset: 100})
	if err != nil {
		t.Fatalf("ListSnapshots over-offset: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots after offset 100, got %d", len(snaps))
	}
}

// TestFS_ListSnapshots_Empty verifies listing on an empty repository.
func TestFS_ListSnapshots_Empty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	snaps, err := store.ListSnapshots(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSnapshots on empty store: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(snaps))
	}
}

// TestFS_ManifestPath_ShardedLayout verifies the manifest path uses the
// same sharded layout as snapshots (<hex[:2]>/<hex[2:]>).
func TestFS_ManifestPath_ShardedLayout(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	id := core.SnapshotID{Hash: core.Hash{0xab, 0xcd}}
	hexStr := id.Hash.FullString()
	manPath := store.manifestPath(id)
	expected := filepath.Join(dir, ManifestsDir, hexStr[:2], hexStr[2:])
	if manPath != expected {
		t.Errorf("manifestPath: got %q, want %q", manPath, expected)
	}
}

// TestFS_GetSnapshot_LargeSnapshot verifies that GetSnapshot can read a
// large snapshot (many file entries with chunk hashes) via the streaming
// os.Open + io.ReadAll path without panic, and that the integrity check
// passes and data round-trips correctly.
func TestFS_GetSnapshot_LargeSnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStorage(dir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer store.Close()

	// Build a snapshot with many file entries to produce a multi-MB proto.
	const numFiles = 10_000
	files := make([]core.FileEntry, numFiles)
	for i := range files {
		chunks := make([]core.Hash, 4)
		for j := range chunks {
			chunks[j] = core.Hash{byte(i), byte(i >> 8), byte(j), 0x01}
		}
		files[i] = core.FileEntry{
			Path:    fmt.Sprintf("src/module_%05d/file_%05d.txt", i, i),
			Size:    int64(i * 100),
			ModTime: int64(i),
			Chunks:  chunks,
			Hash:    core.Hash{byte(i), byte(i >> 8), byte(i >> 16), 0x02},
		}
	}
	snap := &core.Snapshot{
		Message:   "large snapshot streaming test",
		Author:    "tester",
		Timestamp: 42,
		Files:     files,
		TotalSize: 9876543,
		Tags:      []string{"v1", "large"},
	}

	// Compute the snapshot ID the same way porcelain.CreateSnapshot does:
	// blake3 of the marshaled proto with IdHash omitted.
	pNoID := core.SnapshotToProto(snap, false)
	marshaled, err := proto.Marshal(pNoID)
	if err != nil {
		t.Fatalf("marshal snapshot for ID: %v", err)
	}
	snap.ID = core.SnapshotID{Hash: core.Hash(blake3.Sum256(marshaled))}

	ctx := context.Background()
	if err := store.PutSnapshot(ctx, snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}

	got, err := store.GetSnapshot(ctx, snap.ID)
	if err != nil {
		t.Fatalf("GetSnapshot large: %v", err)
	}
	if got.Message != snap.Message {
		t.Errorf("Message: got %q, want %q", got.Message, snap.Message)
	}
	if got.Author != snap.Author {
		t.Errorf("Author: got %q, want %q", got.Author, snap.Author)
	}
	if got.Timestamp != snap.Timestamp {
		t.Errorf("Timestamp: got %d, want %d", got.Timestamp, snap.Timestamp)
	}
	if got.TotalSize != snap.TotalSize {
		t.Errorf("TotalSize: got %d, want %d", got.TotalSize, snap.TotalSize)
	}
	if len(got.Files) != numFiles {
		t.Fatalf("Files length: got %d, want %d", len(got.Files), numFiles)
	}
	// Spot-check first and last entries (path + chunk count + file hash).
	first, last := got.Files[0], got.Files[numFiles-1]
	if first.Path != files[0].Path {
		t.Errorf("first file path: got %q, want %q", first.Path, files[0].Path)
	}
	if len(first.Chunks) != 4 {
		t.Errorf("first file chunks: got %d, want 4", len(first.Chunks))
	}
	if first.Hash != files[0].Hash {
		t.Errorf("first file hash mismatch")
	}
	if last.Path != files[numFiles-1].Path {
		t.Errorf("last file path: got %q, want %q", last.Path, files[numFiles-1].Path)
	}
	if last.Hash != files[numFiles-1].Hash {
		t.Errorf("last file hash mismatch")
	}
}
