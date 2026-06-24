package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/drift/drift/internal/core"
)

// newTestStore returns a Store rooted in a fresh temp directory, already initialized.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return s
}

// TestStore_Init_CreatesDirs verifies that Init creates the expected directory layout.
func TestStore_Init_CreatesDirs(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	expected := []string{
		".drift",
		".drift/objects/blobs",
		".drift/objects/trees",
		".drift/commits",
		".drift/refs",
	}
	for _, rel := range expected {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".drift", "config.json")); err != nil {
		t.Fatalf("expected config.json to exist: %v", err)
	}
}

// TestStore_Init_Idempotent verifies that calling Init twice does not error.
func TestStore_Init_Idempotent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}
	if err := s.Init(); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
}

// TestStore_IsInitialized verifies detection of an initialized project.
func TestStore_IsInitialized(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if s.IsInitialized() {
		t.Fatal("expected not initialized before Init")
	}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if !s.IsInitialized() {
		t.Fatal("expected initialized after Init")
	}
}

// TestStore_PutBlob_Deduplicates verifies that identical content is stored once.
func TestStore_PutBlob_Deduplicates(t *testing.T) {
	s := newTestStore(t)
	h1, err := s.PutBlob([]byte("hello"))
	if err != nil {
		t.Fatalf("PutBlob failed: %v", err)
	}
	h2, err := s.PutBlob([]byte("hello"))
	if err != nil {
		t.Fatalf("second PutBlob failed: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected identical hashes, got %q and %q", h1, h2)
	}
}

// TestStore_GetBlob_RoundTrip verifies that a stored blob can be retrieved.
func TestStore_GetBlob_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	hash, err := s.PutBlob([]byte("drift"))
	if err != nil {
		t.Fatalf("PutBlob failed: %v", err)
	}
	got, err := s.GetBlob(hash)
	if err != nil {
		t.Fatalf("GetBlob failed: %v", err)
	}
	if string(got) != "drift" {
		t.Fatalf("got %q, want drift", string(got))
	}
}

// TestStore_GetBlob_NotFound verifies that a missing blob returns ErrObjectNotFound.
func TestStore_GetBlob_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetBlob("0000000000000000000000000000000000000000000000000000000000000000")
	if err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

// TestStore_GetBlob_Corrupted verifies that a tampered blob returns ErrObjectCorrupted.
func TestStore_GetBlob_Corrupted(t *testing.T) {
	s := newTestStore(t)
	hash, err := s.PutBlob([]byte("drift"))
	if err != nil {
		t.Fatal(err)
	}
	// Overwrite the blob file with valid compressed data but different content.
	path := s.blobPath(hash)
	compressed, err := compressBytes([]byte("tampered"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, compressed, 0644); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetBlob(hash)
	if err != ErrObjectCorrupted {
		t.Fatalf("expected ErrObjectCorrupted, got %v", err)
	}
}

// TestStore_PutBlobFromFile verifies that a file is streamed into the store with the correct hash.
func TestStore_PutBlobFromFile(t *testing.T) {
	s := newTestStore(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "input.bin")
	data := []byte("file content")
	if err := os.WriteFile(src, data, 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := s.PutBlobFromFile(src)
	if err != nil {
		t.Fatalf("PutBlobFromFile failed: %v", err)
	}
	want := core.CalculateHash(data)
	if hash != want {
		t.Fatalf("hash mismatch: got %q, want %q", hash, want)
	}
	// The blob should be retrievable.
	got, err := s.GetBlob(hash)
	if err != nil {
		t.Fatalf("GetBlob failed: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("content mismatch: got %q, want %q", string(got), string(data))
	}
}

// TestStore_PutBlobFromFile_LargeFile verifies that large files do not cause OOM (stream hashing).
func TestStore_PutBlobFromFile_LargeFile(t *testing.T) {
	s := newTestStore(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "big.bin")
	data := make([]byte, 5*1024*1024) // 5 MB
	for i := range data {
		data[i] = byte(i % 251)
	}
	if err := os.WriteFile(src, data, 0644); err != nil {
		t.Fatal(err)
	}
	hash, err := s.PutBlobFromFile(src)
	if err != nil {
		t.Fatalf("PutBlobFromFile failed: %v", err)
	}
	if hash != core.CalculateHash(data) {
		t.Fatal("large file hash mismatch")
	}
}

// TestStore_PutBlobFromFile_MissingSource verifies that a missing source file returns an error.
func TestStore_PutBlobFromFile_MissingSource(t *testing.T) {
	s := newTestStore(t)
	_, err := s.PutBlobFromFile(filepath.Join(t.TempDir(), "nope.bin"))
	if err == nil {
		t.Fatal("expected error for missing source file, got nil")
	}
}

// TestStore_PutBlobFromFile_TmpCleanup verifies that the tmp file is removed on success.
func TestStore_PutBlobFromFile_TmpCleanup(t *testing.T) {
	s := newTestStore(t)
	src := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(src, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutBlobFromFile(src); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(s.DriftDir(), "objects/blobs", ".puttmp")
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected tmp file to be removed, got err=%v", err)
	}
}

// TestStore_PutTree_GetTree verifies tree storage and retrieval.
func TestStore_PutTree_GetTree(t *testing.T) {
	s := newTestStore(t)
	tree, err := core.NewTree([]core.TreeEntry{
		{Name: "a.txt", Type: core.BlobObject, Hash: "0000000000000000000000000000000000000000000000000000000000000001", Mode: core.ModeRegular},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutTree(tree); err != nil {
		t.Fatalf("PutTree failed: %v", err)
	}
	got, err := s.GetTree(tree.Hash)
	if err != nil {
		t.Fatalf("GetTree failed: %v", err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Name != "a.txt" {
		t.Fatalf("tree mismatch: %+v", got)
	}
}

// TestStore_PutTree_Idempotent verifies that storing the same tree twice does not error.
func TestStore_PutTree_Idempotent(t *testing.T) {
	s := newTestStore(t)
	tree, err := core.NewTree([]core.TreeEntry{
		{Name: "a", Type: core.BlobObject, Hash: "0000000000000000000000000000000000000000000000000000000000000001", Mode: core.ModeRegular},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutTree(tree); err != nil {
		t.Fatal(err)
	}
	if err := s.PutTree(tree); err != nil {
		t.Fatalf("second PutTree failed: %v", err)
	}
}

// TestStore_GetTree_NotFound verifies that a missing tree returns ErrObjectNotFound.
func TestStore_GetTree_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetTree("0000000000000000000000000000000000000000000000000000000000000000")
	if err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

// TestStore_PutCommit_GetCommit verifies commit storage and retrieval.
func TestStore_PutCommit_GetCommit(t *testing.T) {
	s := newTestStore(t)
	c := core.NewCommit("v1", "msg", "", "main",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		core.Signature{Name: "alice", Email: "a@b.c"})
	if err := s.PutCommit(c); err != nil {
		t.Fatalf("PutCommit failed: %v", err)
	}
	// GetCommit now uses hash as the file identifier
	got, err := s.GetCommit(c.Hash)
	if err != nil {
		t.Fatalf("GetCommit failed: %v", err)
	}
	if got.Hash != c.Hash || got.Message != c.Message {
		t.Fatalf("commit mismatch: got %+v, want %+v", got, c)
	}
}

// TestStore_GetCommit_NotFound verifies that a missing commit returns ErrObjectNotFound.
func TestStore_GetCommit_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetCommit("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

// TestStore_ListCommits_OrderedByTimestamp verifies that ListCommits returns commits sorted by timestamp.
func TestStore_ListCommits_OrderedByTimestamp(t *testing.T) {
	s := newTestStore(t)
	treeHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	older := core.NewCommit("v1", "old", "", "main", treeHash, core.Signature{Name: "a", Email: "b"})
	if err := s.PutCommit(older); err != nil {
		t.Fatal(err)
	}

	// Sleep to ensure newer commit has a strictly later timestamp.
	time.Sleep(2 * time.Millisecond)
	newer := core.NewCommit("v2", "new", older.Hash, "main", treeHash, core.Signature{Name: "a", Email: "b"})
	if err := s.PutCommit(newer); err != nil {
		t.Fatal(err)
	}

	commits, err := s.ListCommits()
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].ID != "v1" {
		t.Fatalf("expected v1 first, got %q", commits[0].ID)
	}
	if commits[1].ID != "v2" {
		t.Fatalf("expected v2 second, got %q", commits[1].ID)
	}
}

// TestStore_SaveRef_GetRef verifies ref storage and retrieval.
func TestStore_SaveRef_GetRef(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveRef("HEAD", "main"); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}
	got, err := s.GetRef("HEAD")
	if err != nil {
		t.Fatalf("GetRef failed: %v", err)
	}
	if got != "main" {
		t.Fatalf("got %q, want main", got)
	}
}

// TestStore_GetRef_NotFound verifies that a missing ref returns ErrObjectNotFound.
func TestStore_GetRef_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetRef("missing")
	if err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

// TestStore_SaveRef_Overwrite verifies that saving a ref twice overwrites the previous value.
func TestStore_SaveRef_Overwrite(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("main", "hash1")
	_ = s.SaveRef("main", "hash2")
	got, _ := s.GetRef("main")
	if got != "hash2" {
		t.Fatalf("expected hash2 after overwrite, got %q", got)
	}
}

// TestStore_ListRefs verifies that ListRefs returns all saved refs.
func TestStore_ListRefs(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("HEAD", "main")
	_ = s.SaveRef("main", "abc")
	_ = s.SaveRef("feature", "def")

	refs, err := s.ListRefs()
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	want := map[string]string{"HEAD": "main", "main": "abc", "feature": "def"}
	if len(refs) != len(want) {
		t.Fatalf("expected %d refs, got %d (%v)", len(want), len(refs), refs)
	}
	for k, v := range want {
		if refs[k] != v {
			t.Fatalf("ref %q = %q, want %q", k, refs[k], v)
		}
	}
}

// TestStore_SaveIndex_LoadIndex verifies index round trip.
func TestStore_SaveIndex_LoadIndex(t *testing.T) {
	s := newTestStore(t)
	idx := &core.Index{}
	idx.Add(core.IndexEntry{
		Path:       "a.txt",
		Hash:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ModifiedAt: time.UnixMilli(1700000000000),
		Size:       42,
		Mode:       core.ModeRegular,
	})
	if err := s.SaveIndex(idx); err != nil {
		t.Fatalf("SaveIndex failed: %v", err)
	}

	got := &core.Index{}
	if err := s.LoadIndex(got); err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got.Entries))
	}
	e, _ := got.Entry("a.txt")
	if e.Hash != "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" {
		t.Fatalf("hash mismatch: %q", e.Hash)
	}
}

// TestStore_LoadIndex_MissingFile verifies that loading a missing index is a no-op (no error).
func TestStore_LoadIndex_MissingFile(t *testing.T) {
	s := newTestStore(t)
	idx := &core.Index{}
	if err := s.LoadIndex(idx); err != nil {
		t.Fatalf("LoadIndex returned error: %v", err)
	}
	if len(idx.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(idx.Entries))
	}
}

// TestStore_Init_WritesConfig verifies that Init writes a config.json with expected fields.
func TestStore_Init_WritesConfig(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".drift", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	// Issue 13: Init now writes the standard config schema (user + core).
	user, ok := cfg["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected user object, got %v", cfg["user"])
	}
	if user["name"] == nil {
		t.Fatalf("expected user.name to be set, got %v", user["name"])
	}
}

// TestStore_PutBlobFromFile_Deduplicates verifies that two identical files produce one stored blob.
func TestStore_PutBlobFromFile_Deduplicates(t *testing.T) {
	s := newTestStore(t)
	src1 := filepath.Join(t.TempDir(), "a.txt")
	src2 := filepath.Join(t.TempDir(), "b.txt")
	_ = os.WriteFile(src1, []byte("same"), 0644)
	_ = os.WriteFile(src2, []byte("same"), 0644)

	h1, _ := s.PutBlobFromFile(src1)
	h2, _ := s.PutBlobFromFile(src2)
	if h1 != h2 {
		t.Fatalf("expected identical hashes, got %q and %q", h1, h2)
	}
}

// TestStore_Lock_Release verifies that the lock is released after the unlock function is called.
func TestStore_Lock_Release(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("file lock behavior differs on Unix; covered by lock tests")
	}
	s := newTestStore(t)
	unlock1, err := s.lock()
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	unlock1()

	// After unlock, a second lock should succeed.
	unlock2, err := s.lock()
	if err != nil {
		t.Fatalf("second lock failed after release: %v", err)
	}
	unlock2()
}

// TestStore_Lock_WritesPID verifies that writeLockPID/readLockPID/clearLockPID
// work correctly. We test the helpers directly because Windows denies reading
// a locked file via a separate file handle.
func TestStore_Lock_WritesPID(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}
	defer f.Close()

	// Write PID.
	writeLockPID(f)
	pid := readLockPID(f)
	if pid != os.Getpid() {
		t.Errorf("readLockPID = %d, want %d", pid, os.Getpid())
	}

	// Clear PID.
	clearLockPID(f)
	pid = readLockPID(f)
	if pid != 0 {
		t.Errorf("readLockPID after clear = %d, want 0", pid)
	}
}

// TestStore_ListCommits_Empty verifies that an empty commits directory returns an empty list.
func TestStore_ListCommits_Empty(t *testing.T) {
	s := newTestStore(t)
	commits, err := s.ListCommits()
	if err != nil {
		t.Fatalf("ListCommits failed: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(commits))
	}
}

// TestStore_ListRefs_Empty verifies that an empty refs directory returns an empty map.
func TestStore_ListRefs_Empty(t *testing.T) {
	s := newTestStore(t)
	refs, err := s.ListRefs()
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(refs))
	}
}

// TestStore_PutBlobFromFile_DoesNotConflictWithExistingBlob verifies that re-adding a file
// whose blob already exists does not error and returns the same hash.
func TestStore_PutBlobFromFile_DoesNotConflictWithExistingBlob(t *testing.T) {
	s := newTestStore(t)
	src := filepath.Join(t.TempDir(), "a.txt")
	_ = os.WriteFile(src, []byte("x"), 0644)

	h1, _ := s.PutBlobFromFile(src)
	h2, _ := s.PutBlobFromFile(src)
	if h1 != h2 {
		t.Fatalf("expected same hash on re-add, got %q and %q", h1, h2)
	}
}

// --- GetBlobSize tests ---

// TestStore_GetBlobSize verifies that GetBlobSize returns the original
// (uncompressed) content size for a stored blob.
func TestStore_GetBlobSize(t *testing.T) {
	s := newTestStore(t)
	data := []byte("hello world")
	hash, err := s.PutBlob(data)
	if err != nil {
		t.Fatalf("PutBlob failed: %v", err)
	}
	size, err := s.GetBlobSize(hash)
	if err != nil {
		t.Fatalf("GetBlobSize failed: %v", err)
	}
	if size != int64(len(data)) {
		t.Fatalf("got size %d, want %d", size, len(data))
	}
}

// TestStore_GetBlobSize_NotFound verifies that a missing blob returns ErrObjectNotFound.
func TestStore_GetBlobSize_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetBlobSize("0000000000000000000000000000000000000000000000000000000000000000")
	if err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

// TestStore_GetBlobSize_LargeFile verifies size correctness for a larger blob
// that is likely to be compressed.
func TestStore_GetBlobSize_LargeFile(t *testing.T) {
	s := newTestStore(t)
	data := make([]byte, 10000)
	for i := range data {
		data[i] = byte(i % 251)
	}
	hash, _ := s.PutBlob(data)
	size, err := s.GetBlobSize(hash)
	if err != nil {
		t.Fatalf("GetBlobSize failed: %v", err)
	}
	if size != int64(len(data)) {
		t.Fatalf("got size %d, want %d", size, len(data))
	}
}

// --- GetBlobToWriter tests ---

// TestStore_GetBlobToWriter_RoundTrip verifies that a blob streamed to a
// writer matches the original content.
func TestStore_GetBlobToWriter_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	data := []byte("streaming content")
	hash, _ := s.PutBlob(data)

	var buf bytes.Buffer
	if err := s.GetBlobToWriter(hash, &buf); err != nil {
		t.Fatalf("GetBlobToWriter failed: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("content mismatch: got %q, want %q", buf.String(), string(data))
	}
}

// TestStore_GetBlobToWriter_NotFound verifies that a missing blob returns ErrObjectNotFound.
func TestStore_GetBlobToWriter_NotFound(t *testing.T) {
	s := newTestStore(t)
	var buf bytes.Buffer
	err := s.GetBlobToWriter("0000000000000000000000000000000000000000000000000000000000000000", &buf)
	if err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

// TestStore_GetBlobToWriter_Corrupted verifies that a tampered blob returns ErrObjectCorrupted.
func TestStore_GetBlobToWriter_Corrupted(t *testing.T) {
	s := newTestStore(t)
	hash, _ := s.PutBlob([]byte("original"))
	path := s.blobPath(hash)
	compressed, err := compressBytes([]byte("tampered"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, compressed, 0644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = s.GetBlobToWriter(hash, &buf)
	if err != ErrObjectCorrupted {
		t.Fatalf("expected ErrObjectCorrupted, got %v", err)
	}
}

// --- PutBlobFromReader tests ---

// TestStore_PutBlobFromReader verifies that content from an io.Reader is stored correctly.
func TestStore_PutBlobFromReader(t *testing.T) {
	s := newTestStore(t)
	data := []byte("from reader")
	hash, err := s.PutBlobFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("PutBlobFromReader failed: %v", err)
	}
	want := core.CalculateHash(data)
	if hash != want {
		t.Fatalf("hash mismatch: got %q, want %q", hash, want)
	}
	got, err := s.GetBlob(hash)
	if err != nil {
		t.Fatalf("GetBlob failed: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("content mismatch: got %q, want %q", string(got), string(data))
	}
}

// --- WithLock tests ---

// TestStore_WithLock verifies that WithLock runs the function and releases the lock.
func TestStore_WithLock(t *testing.T) {
	s := newTestStore(t)
	called := false
	err := s.WithLock(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithLock failed: %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called")
	}
	// After WithLock returns, a second lock should succeed (lock was released).
	unlock, err := s.lock()
	if err != nil {
		t.Fatalf("lock after WithLock failed: %v", err)
	}
	unlock()
}

// TestStore_WithLock_PropagatesError verifies that errors from fn are returned.
func TestStore_WithLock_PropagatesError(t *testing.T) {
	s := newTestStore(t)
	err := s.WithLock(func() error {
		return fmt.Errorf("custom error")
	})
	if err == nil || err.Error() != "custom error" {
		t.Fatalf("expected custom error, got %v", err)
	}
}

// --- DeleteRef tests ---

// TestStore_DeleteRef verifies that a ref is deleted.
func TestStore_DeleteRef(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("feature", "abc123")
	if err := s.DeleteRef("feature"); err != nil {
		t.Fatalf("DeleteRef failed: %v", err)
	}
	_, err := s.GetRef("feature")
	if err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound after delete, got %v", err)
	}
}

// TestStore_DeleteRef_HEAD verifies that deleting HEAD is refused.
func TestStore_DeleteRef_HEAD(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteRef("HEAD")
	if err == nil {
		t.Fatal("expected error deleting HEAD, got nil")
	}
}

// TestStore_DeleteRef_NotFound verifies that deleting a nonexistent ref errors.
func TestStore_DeleteRef_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteRef("nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent ref, got nil")
	}
}

// --- RenameRef tests ---

// TestStore_RenameRef verifies that a ref is renamed correctly.
func TestStore_RenameRef(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("old", "hash123")
	if err := s.RenameRef("old", "new"); err != nil {
		t.Fatalf("RenameRef failed: %v", err)
	}
	got, err := s.GetRef("new")
	if err != nil {
		t.Fatalf("GetRef(new) failed: %v", err)
	}
	if got != "hash123" {
		t.Fatalf("got %q, want hash123", got)
	}
	// Old ref should be gone.
	_, err = s.GetRef("old")
	if err != ErrObjectNotFound {
		t.Fatalf("expected old ref to be deleted, got err=%v", err)
	}
}

// TestStore_RenameRef_HEAD verifies that renaming HEAD is refused.
func TestStore_RenameRef_HEAD(t *testing.T) {
	s := newTestStore(t)
	err := s.RenameRef("HEAD", "main")
	if err == nil {
		t.Fatal("expected error renaming HEAD, got nil")
	}
}

// TestStore_RenameRef_TargetExists verifies that renaming to an existing ref errors.
func TestStore_RenameRef_TargetExists(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("a", "hash1")
	_ = s.SaveRef("b", "hash2")
	err := s.RenameRef("a", "b")
	if err == nil {
		t.Fatal("expected error renaming to existing ref, got nil")
	}
}

// TestStore_RenameRef_UpdatesHEAD verifies that HEAD is updated when renaming
// the branch it points to.
func TestStore_RenameRef_UpdatesHEAD(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("HEAD", "old")
	_ = s.SaveRef("old", "hash123")
	if err := s.RenameRef("old", "new"); err != nil {
		t.Fatalf("RenameRef failed: %v", err)
	}
	head, _ := s.GetRef("HEAD")
	if head != "new" {
		t.Fatalf("HEAD = %q, want new", head)
	}
}

// TestStore_RenameRef_SameName verifies that renaming to the same name is a no-op.
func TestStore_RenameRef_SameName(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("branch", "hash123")
	if err := s.RenameRef("branch", "branch"); err != nil {
		t.Fatalf("RenameRef same name failed: %v", err)
	}
}

// --- SaveCommitTransaction tests ---

// TestStore_SaveCommitTransaction verifies that a commit, branch ref, and index
// are all persisted atomically.
func TestStore_SaveCommitTransaction(t *testing.T) {
	s := newTestStore(t)
	treeHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	c := core.NewCommit("v1", "test commit", "", "main", treeHash, core.Signature{Name: "a", Email: "b"})

	idx := &core.Index{}
	idx.Add(core.IndexEntry{
		Path:       "file.txt",
		Hash:       treeHash,
		ModifiedAt: time.UnixMilli(1700000000000),
		Size:       42,
		Mode:       core.ModeRegular,
	})

	if err := s.SaveCommitTransaction(c, "main", idx); err != nil {
		t.Fatalf("SaveCommitTransaction failed: %v", err)
	}

	// Commit should be retrievable.
	got, err := s.GetCommit(c.Hash)
	if err != nil {
		t.Fatalf("GetCommit failed: %v", err)
	}
	if got.ID != "v1" {
		t.Fatalf("commit ID = %q, want v1", got.ID)
	}

	// Branch ref should point to the commit hash.
	ref, err := s.GetRef("main")
	if err != nil {
		t.Fatalf("GetRef failed: %v", err)
	}
	if ref != c.Hash {
		t.Fatalf("ref = %q, want %q", ref, c.Hash)
	}

	// Index should be persisted with the entry.
	loaded := &core.Index{}
	if err := s.LoadIndex(loaded); err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("expected 1 index entry, got %d", len(loaded.Entries))
	}
}

// --- ListBranchCommits tests ---

// TestStore_ListBranchCommits verifies that commits are returned newest-first
// by walking the parent chain.
func TestStore_ListBranchCommits(t *testing.T) {
	s := newTestStore(t)
	treeHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	c1 := core.NewCommit("v1", "first", "", "main", treeHash, core.Signature{Name: "a", Email: "b"})
	_ = s.PutCommit(c1)
	_ = s.SaveRef("main", c1.Hash)

	time.Sleep(2 * time.Millisecond)
	c2 := core.NewCommit("v2", "second", c1.Hash, "main", treeHash, core.Signature{Name: "a", Email: "b"})
	_ = s.PutCommit(c2)
	_ = s.SaveRef("main", c2.Hash)

	commits, err := s.ListBranchCommits("main")
	if err != nil {
		t.Fatalf("ListBranchCommits failed: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}
	if commits[0].ID != "v2" {
		t.Fatalf("first commit = %q, want v2 (newest first)", commits[0].ID)
	}
	if commits[1].ID != "v1" {
		t.Fatalf("second commit = %q, want v1", commits[1].ID)
	}
}

// TestStore_ListBranchCommits_EmptyBranch verifies that a branch with no commits
// (empty ref) returns an empty slice.
func TestStore_ListBranchCommits_EmptyBranch(t *testing.T) {
	s := newTestStore(t)
	_ = s.SaveRef("main", "")
	commits, err := s.ListBranchCommits("main")
	if err != nil {
		t.Fatalf("ListBranchCommits failed: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(commits))
	}
}

// TestStore_ListBranchCommits_NonexistentBranch verifies that a nonexistent
// branch returns an empty slice (not an error).
func TestStore_ListBranchCommits_NonexistentBranch(t *testing.T) {
	s := newTestStore(t)
	commits, err := s.ListBranchCommits("nonexistent")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent branch, got %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(commits))
	}
}

// keep imports sorted for stability across edits
func init() {
	_ = sort.Strings
}
