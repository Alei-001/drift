package storage

import (
	"encoding/json"
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
	// Overwrite the blob file with different content.
	path := s.blobPath(hash)
	if err := os.WriteFile(path, []byte("tampered"), 0644); err != nil {
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
		{Name: "a.txt", Type: core.BlobObject, Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: core.ModeRegular},
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
	tree, _ := core.NewTree([]core.TreeEntry{
		{Name: "a", Type: core.BlobObject, Hash: "0000000000000000000000000000000000000000000000000000000000000000", Mode: core.ModeRegular},
	})
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
	_, err := s.GetCommit("missing")
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

// keep imports sorted for stability across edits
func init() {
	_ = sort.Strings
}
