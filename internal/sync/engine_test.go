package sync

import (
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// memTransport is an in-memory Transport for testing the sync engine
// without touching the filesystem or network.
type memTransport struct {
	files map[string][]byte
	dirs  map[string]bool
}

func newMemTransport() *memTransport {
	return &memTransport{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (t *memTransport) Get(remotePath string, dst io.Writer) error {
	data, ok := t.files[remotePath]
	if !ok {
		return os.ErrNotExist
	}
	_, err := dst.Write(data)
	return err
}

func (t *memTransport) Put(remotePath string, src io.Reader) error {
	data, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	t.files[remotePath] = data
	// Mark parent dirs as existing.
	dir := remotePath
	for {
		dir = path.Dir(dir)
		if dir == "." || dir == "/" {
			break
		}
		t.dirs[dir] = true
	}
	return nil
}

func (t *memTransport) Stat(remotePath string) (*RemoteStat, error) {
	data, ok := t.files[remotePath]
	if !ok {
		return nil, os.ErrNotExist
	}
	return &RemoteStat{Size: int64(len(data))}, nil
}

func (t *memTransport) List(prefix string) ([]string, error) {
	var files []string
	for path := range t.files {
		if prefix == "" || strings.HasPrefix(path, prefix) {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files, nil
}

func (t *memTransport) Delete(remotePath string) error {
	delete(t.files, remotePath)
	return nil
}

func (t *memTransport) Mkdir(remotePath string) error {
	t.dirs[remotePath] = true
	return nil
}

func (t *memTransport) Close() error { return nil }

// --- Engine tests ---

// TestEngine_FirstSync_PushesAllFiles tests that the first sync pushes all
// local files to an empty remote.
func TestEngine_FirstSync_PushesAllFiles(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "a.txt", "content a")
	writeLocalFile(t, localDir, "b.txt", "content b")
	writeLocalFile(t, localDir, "sub/c.txt", "content c")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project-id")

	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if len(result.Pushed) != 3 {
		t.Errorf("expected 3 pushed files, got %d: %v", len(result.Pushed), result.Pushed)
	}
	if len(result.Pulled) != 0 {
		t.Errorf("expected 0 pulled files, got %d", len(result.Pulled))
	}

	// Verify files are on the remote.
	if _, ok := transport.files["a.txt"]; !ok {
		t.Error("a.txt not pushed to remote")
	}
	if _, ok := transport.files["b.txt"]; !ok {
		t.Error("b.txt not pushed to remote")
	}
	if _, ok := transport.files["sub/c.txt"]; !ok {
		t.Error("sub/c.txt not pushed to remote")
	}

	// Verify manifest was saved.
	manifestData, ok := transport.files[manifestPath]
	if !ok {
		t.Fatal("manifest not saved on remote")
	}
	if !bytes.Contains(manifestData, []byte("test-project-id")) {
		t.Errorf("manifest doesn't contain project ID: %s", manifestData)
	}
}

// TestEngine_SecondSync_NoChanges tests that a second sync with no changes
// is a no-op.
func TestEngine_SecondSync_NoChanges(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "a.txt", "content a")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	// First sync.
	_, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// Second sync — should be no-op.
	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasChanges() {
		t.Errorf("expected no changes on second sync, got: %+v", result)
	}
}

// TestEngine_IncrementalSync_OnlyChangedFiles tests that only changed files
// are transferred on subsequent syncs.
func TestEngine_IncrementalSync_OnlyChangedFiles(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "a.txt", "original a")
	writeLocalFile(t, localDir, "b.txt", "original b")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	// First sync — both files pushed.
	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pushed) != 2 {
		t.Fatalf("expected 2 pushed on first sync, got %d", len(result.Pushed))
	}

	// Modify only a.txt.
	writeLocalFile(t, localDir, "a.txt", "modified a")

	// Second sync — only a.txt should be pushed.
	result, err = engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pushed) != 1 {
		t.Errorf("expected 1 pushed on second sync, got %d: %v", len(result.Pushed), result.Pushed)
	}
	if len(result.Pushed) > 0 && result.Pushed[0] != "a.txt" {
		t.Errorf("expected a.txt to be pushed, got %s", result.Pushed[0])
	}
}

// TestEngine_PullNewFilesFromRemote tests that files added on the remote are
// pulled to local.
func TestEngine_PullNewFilesFromRemote(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "a.txt", "content a")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	// First sync.
	_, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// Add a file directly on the remote (simulating another device pushing).
	transport.files["remote-only.txt"] = []byte("from remote")

	// Second sync — should pull the new file.
	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pulled) != 1 {
		t.Errorf("expected 1 pulled, got %d: %v", len(result.Pulled), result.Pulled)
	}

	// Verify file was pulled to local.
	data, err := os.ReadFile(filepath.Join(localDir, "remote-only.txt"))
	if err != nil {
		t.Fatalf("pulled file not found locally: %v", err)
	}
	if string(data) != "from remote" {
		t.Errorf("expected 'from remote', got %q", string(data))
	}
}

// TestEngine_DeletionTracking_Push tests that files deleted locally are
// deleted on the remote.
func TestEngine_DeletionTracking_Push(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "a.txt", "content a")
	writeLocalFile(t, localDir, "b.txt", "content b")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	// First sync.
	_, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// Delete a.txt locally.
	if err := os.Remove(filepath.Join(localDir, "a.txt")); err != nil {
		t.Fatal(err)
	}

	// Second sync — a.txt should be deleted on remote.
	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RemoteDeleted) != 1 {
		t.Errorf("expected 1 remote deletion, got %d: %v", len(result.RemoteDeleted), result.RemoteDeleted)
	}
	if len(result.RemoteDeleted) > 0 && result.RemoteDeleted[0] != "a.txt" {
		t.Errorf("expected a.txt to be deleted, got %s", result.RemoteDeleted[0])
	}

	// Verify a.txt is gone from remote.
	if _, ok := transport.files["a.txt"]; ok {
		t.Error("a.txt still exists on remote after deletion sync")
	}
}

// TestEngine_DeletionTracking_Pull tests that files deleted on the remote are
// deleted locally.
func TestEngine_DeletionTracking_Pull(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "a.txt", "content a")
	writeLocalFile(t, localDir, "b.txt", "content b")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	// First sync.
	_, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// Delete a.txt on the remote (simulating another device deleting).
	delete(transport.files, "a.txt")

	// Second sync — a.txt should be deleted locally.
	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.LocalDeleted) != 1 {
		t.Errorf("expected 1 local deletion, got %d: %v", len(result.LocalDeleted), result.LocalDeleted)
	}

	// Verify a.txt is gone locally.
	if _, err := os.Stat(filepath.Join(localDir, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("expected a.txt to be deleted locally, got err: %v", err)
	}
}

// TestEngine_SkipsLockFile tests that .drift/lock is never synced.
func TestEngine_SkipsLockFile(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, ".drift/lock", "lock-content")
	writeLocalFile(t, localDir, "a.txt", "content a")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	_, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// Lock file should not be on remote.
	if _, ok := transport.files[".drift/lock"]; ok {
		t.Error(".drift/lock was synced to remote")
	}
}

// TestEngine_SkipsSyncManifest tests that the sync manifest directory is
// not included in local file scans (only the remote copy should exist).
func TestEngine_SkipsSyncManifest(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "a.txt", "content a")
	// Create a file in .drift/sync/ that shouldn't be synced.
	writeLocalFile(t, localDir, ".drift/sync/local-state.json", "{}")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	_, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// The local sync state file should not be pushed.
	if _, ok := transport.files[".drift/sync/local-state.json"]; ok {
		t.Error(".drift/sync/ file was synced to remote")
	}
}

// TestEngine_BidirectionalSync tests simultaneous push and pull.
func TestEngine_BidirectionalSync(t *testing.T) {
	localDir := t.TempDir()
	writeLocalFile(t, localDir, "local-only.txt", "local")
	writeLocalFile(t, localDir, "shared.txt", "original")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	// First sync.
	_, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// Modify shared.txt locally and add a remote-only file.
	writeLocalFile(t, localDir, "shared.txt", "locally modified")
	transport.files["remote-only.txt"] = []byte("remote")

	// Second sync — should push shared.txt and pull remote-only.txt.
	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatal(err)
	}

	// Note: shared.txt is modified locally, so it's pushed (local wins).
	foundPush := false
	for _, p := range result.Pushed {
		if p == "shared.txt" {
			foundPush = true
		}
	}
	if !foundPush {
		t.Errorf("expected shared.txt to be pushed, got: %v", result.Pushed)
	}

	foundPull := false
	for _, p := range result.Pulled {
		if p == "remote-only.txt" {
			foundPull = true
		}
	}
	if !foundPull {
		t.Errorf("expected remote-only.txt to be pulled, got: %v", result.Pulled)
	}

	// Verify remote-only.txt was pulled.
	data, err := os.ReadFile(filepath.Join(localDir, "remote-only.txt"))
	if err != nil {
		t.Fatalf("remote-only.txt not pulled: %v", err)
	}
	if string(data) != "remote" {
		t.Errorf("expected 'remote', got %q", string(data))
	}
}

// TestEngine_EmptyProject tests syncing an empty project directory.
func TestEngine_EmptyProject(t *testing.T) {
	localDir := t.TempDir()
	// Create .drift/ to make it look like a project, but no files.
	writeLocalFile(t, localDir, ".drift/config.json", "{}")

	transport := newMemTransport()
	engine := NewEngine(transport, "test-project")

	result, err := engine.Sync(localDir)
	if err != nil {
		t.Fatalf("sync of empty project failed: %v", err)
	}
	// Only .drift/config.json should be pushed.
	if len(result.Pushed) != 1 {
		t.Errorf("expected 1 pushed file, got %d: %v", len(result.Pushed), result.Pushed)
	}
}

// --- Helper functions ---

func writeLocalFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
