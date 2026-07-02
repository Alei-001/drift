package porcelain

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/your-org/drift/chunker"
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

// TestSaveTag_AlreadyExists verifies that creating a tag whose name is
// already taken returns ErrTagAlreadyExists. The workspace lock held inside
// SaveTag makes the existence check + write atomic, so a second caller that
// observes the tag (after the first releases the lock) sees it exists.
func TestSaveTag_AlreadyExists(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	var snap1Hash core.Hash
	snap1Hash[0] = 0xAA

	if err := SaveTag(context.Background(), store, dir, "v1", snap1Hash); err != nil {
		t.Fatalf("first SaveTag failed: %v", err)
	}

	err := SaveTag(context.Background(), store, dir, "v1", snap1Hash)
	if !errors.Is(err, ErrTagAlreadyExists) {
		t.Fatalf("expected ErrTagAlreadyExists on second SaveTag, got %v", err)
	}

	// The original tag must be untouched.
	ref, err := store.GetRef(context.Background(), "tags/v1")
	if err != nil {
		t.Fatalf("GetRef tags/v1: %v", err)
	}
	if ref.Target != snap1Hash {
		t.Errorf("tag target was clobbered: got %x, want %x", ref.Target, snap1Hash)
	}
}

// TestSaveTag_ConcurrentSameName is a regression test for the TOCTOU race in
// SaveTag: two goroutines creating the same tag name simultaneously must not
// both succeed. Before the workspace lock was added, both could pass the
// existence check and the second would silently overwrite the first. With the
// lock, the calls serialize: exactly one succeeds, and the loser returns an
// error (ErrTagAlreadyExists if it ran after the winner released the lock, or
// ErrLocked if it contended on the lock while the winner held it). Either way
// no double-create / overwrite occurs.
func TestSaveTag_ConcurrentSameName(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	var snap1Hash, snap2Hash core.Hash
	snap1Hash[0] = 0x11
	snap2Hash[0] = 0x22

	var (
		wg     sync.WaitGroup
		start  = make(chan struct{})
		err1   error
		err2   error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		err1 = SaveTag(context.Background(), store, dir, "v1", snap1Hash)
	}()
	go func() {
		defer wg.Done()
		<-start
		err2 = SaveTag(context.Background(), store, dir, "v1", snap2Hash)
	}()
	close(start)
	wg.Wait()

	// Exactly one must succeed; both succeeding is the TOCTOU bug.
	successCount := 0
	if err1 == nil {
		successCount++
	}
	if err2 == nil {
		successCount++
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one success, got %d (err1=%v, err2=%v)", successCount, err1, err2)
	}

	// The loser must return an error that signals the tag is taken or the
	// workspace is locked — never a silent overwrite.
	var loserErr error
	if err1 != nil {
		loserErr = err1
	} else {
		loserErr = err2
	}
	if !errors.Is(loserErr, ErrTagAlreadyExists) && !errors.Is(loserErr, ErrLocked) {
		t.Fatalf("expected loser error to be ErrTagAlreadyExists or ErrLocked, got %v", loserErr)
	}

	// The stored tag must point at the winner's hash, not be clobbered.
	ref, err := store.GetRef(context.Background(), "tags/v1")
	if err != nil {
		t.Fatalf("GetRef tags/v1: %v", err)
	}
	if ref.Target != snap1Hash && ref.Target != snap2Hash {
		t.Errorf("tag target %x does not match either snapshot hash", ref.Target)
	}
}

// nilChunkerEngine is a test engine whose ChunkerFor always returns nil,
// simulating whole-file chunking. Used to test the large-file guard.
type nilChunkerEngine struct{}

func (nilChunkerEngine) Name() string                                                 { return "nil-test" }
func (nilChunkerEngine) DetectByMagic([]byte) bool                                    { return false }
func (nilChunkerEngine) DetectByExtension(string) bool                                { return false }
func (nilChunkerEngine) DetectByHeuristic(string, []byte) bool                        { return false }
func (nilChunkerEngine) ChunkerFor(int64, *core.CoreConfig) chunker.Chunker           { return nil }
func (nilChunkerEngine) Diff(string, io.Reader, string, io.Reader) (string, error)    { return "", nil }
func (nilChunkerEngine) Preview([]byte, int64, io.Reader, int) (string, error)        { return "", nil }

// TestChunkFile_NilChunkerLargeFile is a regression test for OOM: when
// ChunkerFor returns nil (whole-file mode) and the file exceeds 64 KB,
// chunkFile must return an error instead of reading the entire file into
// memory. Before the fix, a 500 MB video would be fully buffered.
func TestChunkFile_NilChunkerLargeFile(t *testing.T) {
	largeSize := int64(128 * 1024) // 128 KB, above the 64 KB threshold
	_, err := chunkFile("bigfile.bin", bytes.NewReader([]byte("x")), nilChunkerEngine{}, largeSize, nil)
	if err == nil {
		t.Fatal("expected error for large file with nil chunker, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' in error, got: %s", err.Error())
	}
}

// TestChunkFile_NilChunkerSmallFile verifies that small files (< 64 KB)
// still work through the nil-chunker whole-file path.
func TestChunkFile_NilChunkerSmallFile(t *testing.T) {
	data := []byte("hello\nworld\n")
	chunks, err := chunkFile("small.txt", bytes.NewReader(data), nilChunkerEngine{}, int64(len(data)), nil)
	if err != nil {
		t.Fatalf("unexpected error for small file: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
}
