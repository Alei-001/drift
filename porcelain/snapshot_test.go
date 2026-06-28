package porcelain

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage/memory"
)

func TestCreateSnapshot_FirstCommit(t *testing.T) {
	store := memory.NewMemoryStorage()
	// Set up initial state: HEAD with empty hash and empty index
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: core.Hash{},
	})
	store.SetIndex(&core.Index{
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

	snap, err := CreateSnapshot(store, dir, "first commit", "test")
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
	headRef, err := store.GetRef("HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.Target != snap.ID.Hash {
		t.Error("HEAD target does not match snapshot ID")
	}

	// Verify snapshot was stored
	stored, err := store.GetSnapshot(snap.ID)
	if err != nil {
		t.Fatalf("GetSnapshot failed: %v", err)
	}
	if stored.Message != snap.Message {
		t.Error("stored snapshot message mismatch")
	}

	// Verify index was updated
	idx, err := store.GetIndex()
	if err != nil {
		t.Fatalf("GetIndex failed: %v", err)
	}
	if len(idx.Entries) != 2 {
		t.Errorf("expected 2 index entries, got %d", len(idx.Entries))
	}
}

func TestCreateSnapshot_SecondCommit(t *testing.T) {
	store := memory.NewMemoryStorage()
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: core.Hash{},
	})
	store.SetIndex(&core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()

	// First commit
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v1"), 0644)
	snap1, err := CreateSnapshot(store, dir, "first commit", "test")
	if err != nil {
		t.Fatalf("first CreateSnapshot failed: %v", err)
	}

	// Modify a file and add a new one
	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("content v2 - modified"), 0644)
	os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("new file"), 0644)

	snap2, err := CreateSnapshot(store, dir, "second commit", "test")
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
	_, err = store.GetSnapshot(snap1.ID)
	if err != nil {
		t.Fatalf("GetSnapshot for first snapshot failed: %v", err)
	}
	_, err = store.GetSnapshot(snap2.ID)
	if err != nil {
		t.Fatalf("GetSnapshot for second snapshot failed: %v", err)
	}

	// HEAD should point to snap2
	headRef, err := store.GetRef("HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if headRef.Target != snap2.ID.Hash {
		t.Error("HEAD target does not match second snapshot ID")
	}
}

func TestCreateSnapshot_NothingChanged(t *testing.T) {
	store := memory.NewMemoryStorage()
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: core.Hash{},
	})
	store.SetIndex(&core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	_, err := CreateSnapshot(store, dir, "first commit", "test")
	if err != nil {
		t.Fatalf("first CreateSnapshot failed: %v", err)
	}

	_, err = CreateSnapshot(store, dir, "second commit", "test")
	if err == nil {
		t.Fatal("expected 'nothing to save' error, got nil")
	}
	if err.Error() != "nothing to save" {
		t.Errorf("expected 'nothing to save', got '%s'", err.Error())
	}
}

func TestCreateSnapshot_DefaultAuthor(t *testing.T) {
	store := memory.NewMemoryStorage()
	store.SetRef("HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: core.Hash{},
	})
	store.SetIndex(&core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content"), 0644)

	snap, err := CreateSnapshot(store, dir, "test", "")
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

	_, err := CreateSnapshot(store, dir, "", "test")
	if err == nil {
		t.Fatal("expected error for empty message, got nil")
	}
	if err.Error() != "message is required" {
		t.Errorf("expected 'message is required', got '%s'", err.Error())
	}
}
