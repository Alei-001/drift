package porcelain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage/memory"
)

func TestCreateSnapshot_FirstCommit(t *testing.T) {
	store := memory.NewMemoryStorage()
	// Set up initial state: HEAD symref -> heads/main, empty index
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"hello.txt":  "Hello World",
		"foo/bar.go": "package bar\n\nfunc Foo() int { return 42 }\n",
	}
	for name, content := range testFiles {
		fullPath := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	snap, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	// Verify snapshot fields
	if snap.Message != "first commit" {
		t.Errorf("expected message 'first commit', got '%s'", snap.Message)
	}
	if snap.Author != "test" {
		t.Errorf("expected author 'test', got '%s'", snap.Author)
	}
	if snap.PrevID != nil {
		t.Error("expected PrevID to be nil for first commit")
	}
	if len(snap.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(snap.Files))
	}
	if snap.ID.Hash.IsZero() {
		t.Error("expected non-zero snapshot ID")
	}
	if snap.TotalSize <= 0 {
		t.Errorf("expected positive TotalSize, got %d", snap.TotalSize)
	}

	// Verify HEAD was updated
	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.Target != snap.ID.Hash {
		t.Error("HEAD target does not match snapshot ID")
	}

	// Verify snapshot was stored
	stored, err := store.GetSnapshot(context.Background(), snap.ID)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if stored.Message != snap.Message {
		t.Error("stored snapshot message mismatch")
	}

	// Verify index was updated
	idx, err := store.GetIndex(context.Background())
	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}
	if len(idx.Entries) != 2 {
		t.Errorf("expected 2 index entries, got %d", len(idx.Entries))
	}
}

func TestCreateSnapshot_SecondCommit(t *testing.T) {
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()

	// First commit
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v1"), 0644)
	snap1, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot failed: %v", err)
	}

	// Modify a file and add a new one
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v2 - modified"), 0644)
	os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("new file"), 0644)

	snap2, err := CreateSnapshot(context.Background(), store, dir, "second commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("second CreateSnapshot failed: %v", err)
	}

	// Verify PrevID links to first snapshot
	if snap2.PrevID == nil {
		t.Fatal("expected PrevID to be set for second commit")
	}
	if snap2.PrevID.Hash != snap1.ID.Hash {
		t.Error("PrevID does not match first snapshot ID")
	}
	if snap2.Message != "second commit" {
		t.Errorf("expected message 'second commit', got '%s'", snap2.Message)
	}

	// Both snapshots should be retrievable
	_, err = store.GetSnapshot(context.Background(), snap1.ID)
	if err != nil {
		t.Fatalf("GetSnapshot for first snapshot failed: %v", err)
	}
	_, err = store.GetSnapshot(context.Background(), snap2.ID)
	if err != nil {
		t.Fatalf("GetSnapshot for second snapshot failed: %v", err)
	}

	// HEAD should point to snap2
	headRef, err := store.GetRef(context.Background(), "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.Target != snap2.ID.Hash {
		t.Error("HEAD target does not match second snapshot ID")
	}
}

func TestCreateSnapshot_NothingChanged(t *testing.T) {
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	_, err := CreateSnapshot(context.Background(), store, dir, "first commit", "test", nil, nil)
	if err != nil {
		t.Fatalf("first CreateSnapshot failed: %v", err)
	}

	_, err = CreateSnapshot(context.Background(), store, dir, "second commit", "test", nil, nil)
	if err == nil {
		t.Fatal("expected 'nothing to save' error, got nil")
	}
	if !errors.Is(err, ErrNothingToSave) {
		t.Errorf("expected ErrNothingToSave, got '%s'", err.Error())
	}
}

func TestCreateSnapshot_DefaultAuthor(t *testing.T) {
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	snap, err := CreateSnapshot(context.Background(), store, dir, "test", "", nil, nil)
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	if snap.Author != "drift" {
		t.Errorf("expected default author 'drift', got '%s'", snap.Author)
	}
}

func TestCreateSnapshot_EmptyMessage(t *testing.T) {
	store := memory.NewMemoryStorage()
	dir := t.TempDir()

	_, err := CreateSnapshot(context.Background(), store, dir, "", "test", nil, nil)
	if err == nil {
		t.Fatal("expected error for empty message, got nil")
	}
	if err.Error() != "message is required" {
		t.Errorf("expected 'message is required', got '%s'", err.Error())
	}
}

// TestComputeFileHash_EmptyFile verifies that empty files of different
// types (text vs binary) produce identical hashes. Without the empty-file
// guard in chunkFile, empty .txt would go through the whole-file single-
// chunk path (TextEngine.ChunkerFor returns nil) while empty .bin would
// go through FastCDC (which skips zero-length data), yielding different
// chunk counts and therefore different file hashes.
func TestComputeFileHash_EmptyFile(t *testing.T) {
	dir := t.TempDir()

	txtPath := filepath.Join(dir, "empty.txt")
	binPath := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(txtPath, []byte{}, 0644); err != nil {
		t.Fatalf("write empty.txt: %v", err)
	}
	if err := os.WriteFile(binPath, []byte{}, 0644); err != nil {
		t.Fatalf("write empty.bin: %v", err)
	}

	txtHash, err := ComputeFileHash(txtPath, nil)
	if err != nil {
		t.Fatalf("ComputeFileHash empty.txt: %v", err)
	}
	binHash, err := ComputeFileHash(binPath, nil)
	if err != nil {
		t.Fatalf("ComputeFileHash empty.bin: %v", err)
	}

	if txtHash != binHash {
		t.Errorf("empty file hash mismatch across engines: txt=%x bin=%x", txtHash, binHash)
	}
}
