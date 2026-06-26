package sync

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

// --- In-memory Transport for testing ---

type memTransport struct {
	objects map[string][]byte
	refs    map[string]string
}

func newMemTransport() *memTransport {
	return &memTransport{
		objects: make(map[string][]byte),
		refs:    make(map[string]string),
	}
}

func (m *memTransport) Get(key string) (io.ReadCloser, error) {
	data, ok := m.objects[key]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *memTransport) Put(key string, data io.Reader) error {
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	m.objects[key] = b
	return nil
}

func (m *memTransport) Exists(key string) (bool, error) {
	_, ok := m.objects[key]
	return ok, nil
}

func (m *memTransport) GetRef(name string) (string, error) {
	return m.refs[name], nil
}

func (m *memTransport) PutRef(name string, hash string) error {
	m.refs[name] = hash
	return nil
}

func (m *memTransport) ListRefs() (map[string]string, error) {
	result := make(map[string]string, len(m.refs))
	for k, v := range m.refs {
		result[k] = v
	}
	return result, nil
}

func (m *memTransport) Close() error { return nil }

// --- Helpers ---

func newTestStore(t *testing.T) *storage.Store {
	t.Helper()
	s := storage.NewStore(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

// makeCommit creates a blob, tree, and commit in the store for a single-file
// snapshot. Returns commit hash, tree hash, blob hash.
func makeCommit(t *testing.T, store *storage.Store, msg, parent string) (commitHash, treeHash, blobHash string) {
	t.Helper()

	content := []byte(msg + "-content")
	blobHash, err := store.PutBlob(content)
	if err != nil {
		t.Fatal(err)
	}

	tree, err := core.NewTree([]core.TreeEntry{
		{Name: "file.txt", Type: core.BlobObject, Hash: blobHash, Mode: core.ModeRegular},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutTree(tree); err != nil {
		t.Fatal(err)
	}

	commit := core.NewCommit(msg, parent, "main", tree.Hash, core.Signature{Name: "test", Email: "t@t.com"})
	if err := store.PutCommit(commit); err != nil {
		t.Fatal(err)
	}

	return commit.Hash, tree.Hash, blobHash
}

// copyLocalToRemote copies all objects from the local store to the memTransport.
func copyLocalToRemote(t *testing.T, local *storage.Store, remote *memTransport, hashes ...string) {
	t.Helper()
	for _, hash := range hashes {
		for _, typ := range []string{"blob", "tree", "commit"} {
			key := objectPath(hash, typ)
			localPath := filepath.Join(local.DriftDir(), key)
			data, err := os.ReadFile(localPath)
			if err != nil {
				continue // wrong type
			}
			remote.objects[key] = data
			break // found matching type
		}
	}
}

// --- Tests ---

func TestPushToEmptyRemote(t *testing.T) {
	store := newTestStore(t)
	remote := newMemTransport()

	commitHash, treeHash, blobHash := makeCommit(t, store, "first", "")
	store.SaveRef("main", commitHash)

	engine := NewEngine(remote, store)
	result, err := engine.Push("main")
	if err != nil {
		t.Fatal(err)
	}
	if result.Pushed < 3 {
		t.Errorf("expected at least 3 pushed, got %d", result.Pushed)
	}

	remoteHash, _ := remote.GetRef("heads/main")
	if remoteHash != commitHash {
		t.Errorf("remote ref = %q, want %q", remoteHash[:8], commitHash[:8])
	}

	trackingHash, _ := store.GetRef(trackingRef("heads/main"))
	if trackingHash != commitHash {
		t.Errorf("tracking ref = %q, want %q", trackingHash[:8], commitHash[:8])
	}

	// All objects should exist on remote.
	for _, hash := range []string{commitHash, treeHash, blobHash} {
		found := false
		for _, typ := range []string{"commit", "tree", "blob"} {
			if _, ok := remote.objects[objectPath(hash, typ)]; ok {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("object %s not found on remote", hash[:8])
		}
	}
}

func TestPushRejectsDivergence(t *testing.T) {
	store := newTestStore(t)
	remote := newMemTransport()

	cA, _, _ := makeCommit(t, store, "first", "")
	cB, _, _ := makeCommit(t, store, "second", cA)
	cC, _, _ := makeCommit(t, store, "third", cB)
	store.SaveRef("main", cC)

	// Tracking ref at A.
	store.SaveRef(trackingRef("heads/main"), cA)

	// Remote has a different commit at main (diverged).
	remote.PutRef("heads/main", "deadbeef" + strings.Repeat("0", 56))

	engine := NewEngine(remote, store)
	_, err := engine.Push("main")
	if err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("expected divergence error, got: %v", err)
	}
}

func TestPushIncremental(t *testing.T) {
	store := newTestStore(t)
	remote := newMemTransport()

	cA, tA, bA := makeCommit(t, store, "first", "")
	cB, _, _ := makeCommit(t, store, "second", cA)
	store.SaveRef("main", cB)

	// Pre-load remote with commit A's objects.
	copyLocalToRemote(t, store, remote, cA, tA, bA)
	remote.PutRef("heads/main", cA)
	store.SaveRef(trackingRef("heads/main"), cA)

	engine := NewEngine(remote, store)
	result, err := engine.Push("main")
	if err != nil {
		t.Fatal(err)
	}

	if result.Pushed < 1 {
		t.Errorf("expected at least 1 new object, got %d", result.Pushed)
	}
}

func TestFetch(t *testing.T) {
	// Build objects in store A, copy to remote.
	storeA := newTestStore(t)
	cA, tA, bA := makeCommit(t, storeA, "first", "")
	// Create a second commit.
	cB, tB, bB := makeCommit(t, storeA, "second", cA)

	remote := newMemTransport()
	copyLocalToRemote(t, storeA, remote, cB, tB, bB)
	copyLocalToRemote(t, storeA, remote, cA, tA, bA)
	remote.PutRef("heads/main", cB)

	// Fetch into a fresh store (no initial objects).
	storeB := newTestStore(t)
	engine := NewEngine(remote, storeB)
	result, err := engine.Fetch("main")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched < 6 {
		t.Errorf("expected at least 6 objects (2 commits + 2 trees + 2 blobs), got %d", result.Fetched)
	}

	// Tracking ref should be set.
	trackingHash, _ := storeB.GetRef(trackingRef("heads/main"))
	if trackingHash != cB {
		t.Errorf("tracking ref = %q, want %q", trackingHash[:8], cB[:8])
	}
}

func TestFetchUpToDate(t *testing.T) {
	store := newTestStore(t)
	remote := newMemTransport()

	cA, tA, bA := makeCommit(t, store, "first", "")
	copyLocalToRemote(t, store, remote, cA, tA, bA)
	remote.PutRef("heads/main", cA)
	fetchedRef := trackingRef("heads/main")
	store.SaveRef(fetchedRef, cA) // already synced

	engine := NewEngine(remote, store)
	result, err := engine.Fetch("main")
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 0 {
		t.Errorf("expected 0 fetched, got %d", result.Fetched)
	}
}

func TestClone(t *testing.T) {
	storeA := newTestStore(t)
	cA, tA, bA := makeCommit(t, storeA, "first", "")
	cB, tB, bB := makeCommit(t, storeA, "second", cA)

	remote := newMemTransport()
	copyLocalToRemote(t, storeA, remote, cB, tB, bB)
	copyLocalToRemote(t, storeA, remote, cA, tA, bA)
	remote.PutRef("heads/main", cB)

	storeB := newTestStore(t)
	engine := NewEngine(remote, storeB)
	if err := engine.Clone(); err != nil {
		t.Fatal(err)
	}

	// All objects should exist locally.
	for _, hash := range []string{cB, tB, bB, cA, tA, bA} {
		if !storeB.HasObject(hash) {
			t.Errorf("object %s not found after clone", hash[:8])
		}
	}

	// Tracking ref for main should be set.
	trackingHash, _ := storeB.GetRef("remotes/origin/heads/main")
	if trackingHash != cB {
		t.Errorf("tracking ref = %q, want %q", trackingHash[:8], cB[:8])
	}
}

func TestPull(t *testing.T) {
	storeA := newTestStore(t)
	cA, tA, bA := makeCommit(t, storeA, "first", "")
	storeA.SaveRef("main", cA)

	remote := newMemTransport()
	copyLocalToRemote(t, storeA, remote, cA, tA, bA)
	remote.PutRef("heads/main", cA)

	storeB := newTestStore(t)
	engine := NewEngine(remote, storeB)
	result, err := engine.Pull("main")
	if err != nil {
		t.Fatal(err)
	}
	if result.Pulled < 3 {
		t.Errorf("expected at least 3 pulled, got %d", result.Pulled)
	}

	// Local ref should be updated.
	localHash, _ := storeB.GetRef("main")
	if localHash != cA {
		t.Errorf("local ref = %q, want %q", localHash[:8], cA[:8])
	}
}

// Ensure memTransport matches Transport at compile time.
var _ Transport = (*memTransport)(nil)

// unused import guard
var _ = time.Now
