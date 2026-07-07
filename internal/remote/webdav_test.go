package remote

import (
	"context"
	"io"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage/backends/memory"
	"golang.org/x/net/webdav"
)

// startWebDAVTestServer starts a local WebDAV server backed by a temp dir.
// Returns the server URL and a cleanup function.
func startWebDAVTestServer(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	srv := &webdav.Handler{
		FileSystem: webdav.Dir(tmpDir),
		LockSystem: webdav.NewMemLS(),
	}
	httpSrv := httptest.NewServer(srv)
	return httpSrv.URL, func() {
		httpSrv.Close()
	}
}

// TestWebDAV_BasicCRUD verifies the WebDAVFS can Stat/Write/Read/List/Remove
// against a real local WebDAV server.
func TestWebDAV_BasicCRUD(t *testing.T) {
	srvURL, cleanup := startWebDAVTestServer(t)
	defer cleanup()

	cfg := RemoteConfig{
		Name: "test",
		Type: "webdav",
		URL:  srvURL,
		User: "test",
		Options: map[string]string{
			"_password": "secret",
		},
	}
	rfs, err := NewWebDAVFS(cfg)
	if err != nil {
		t.Fatalf("NewWebDAVFS: %v", err)
	}
	defer rfs.Close()

	// Write a file.
	if err := rfs.Write("testdir/hello.txt", strings.NewReader("hello webdav")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Stat it.
	info, err := rfs.Stat("testdir/hello.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size != int64(len("hello webdav")) {
		t.Errorf("Size = %d, want %d", info.Size, len("hello webdav"))
	}
	if info.IsDir {
		t.Error("expected file, not dir")
	}

	// Read it back.
	rc, err := rfs.Read("testdir/hello.txt")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "hello webdav" {
		t.Errorf("Read = %q, want %q", string(data), "hello webdav")
	}

	// List the directory.
	entries, err := rfs.List("testdir")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("List returned %d entries, want 1", len(entries))
	}

	// Remove it.
	if err := rfs.Remove("testdir/hello.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Stat should now return ErrNotExist.
	_, err = rfs.Stat("testdir/hello.txt")
	if err == nil || !strings.Contains(err.Error(), "not exist") {
		t.Errorf("expected ErrNotExist after remove, got %v", err)
	}
}

// TestWebDAV_PushPullEndToEnd verifies the full push/pull flow over a real
// WebDAV server (not the mock). This catches wire-format mismatches that
// mock-only tests miss.
func TestWebDAV_PushPullEndToEnd(t *testing.T) {
	srvURL, cleanup := startWebDAVTestServer(t)
	defer cleanup()

	cfg := RemoteConfig{
		Name: "test",
		Type: "webdav",
		URL:  srvURL,
		User: "test",
		Options: map[string]string{
			"_password": "secret",
		},
	}

	// Source store: has a snapshot + chunk.
	srcStore := memory.NewMemoryStorage()
	defer srcStore.Close()
	snapID, chunkHash := makeTestSnapshot(t, srcStore, "webdav e2e", nil)
	setupBranchRef(t, srcStore, "main", snapID.Hash)

	// Push from source to WebDAV remote.
	rfsPush, err := NewWebDAVFS(cfg)
	if err != nil {
		t.Fatalf("NewWebDAVFS (push): %v", err)
	}
	defer rfsPush.Close()

	stats, err := Push(context.Background(), srcStore, rfsPush, "")
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if stats.SnapshotsUploaded != 1 || stats.ChunksUploaded != 1 {
		t.Fatalf("expected 1 snap + 1 chunk uploaded, got snap=%d chunk=%d",
			stats.SnapshotsUploaded, stats.ChunksUploaded)
	}

	// Pull to a fresh destination store.
	dstStore := memory.NewMemoryStorage()
	defer dstStore.Close()

	rfsPull, err := NewWebDAVFS(cfg)
	if err != nil {
		t.Fatalf("NewWebDAVFS (pull): %v", err)
	}
	defer rfsPull.Close()

	stats, err = Pull(context.Background(), dstStore, rfsPull, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.SnapshotsUploaded != 1 || stats.ChunksUploaded != 1 {
		t.Fatalf("expected 1 snap + 1 chunk downloaded, got snap=%d chunk=%d",
			stats.SnapshotsUploaded, stats.ChunksUploaded)
	}

	// Verify snapshot content in destination.
	snap, err := dstStore.GetSnapshot(context.Background(), snapID)
	if err != nil {
		t.Fatalf("GetSnapshot on dst: %v", err)
	}
	if snap.Message != "webdav e2e" {
		t.Errorf("snap.Message = %q, want %q", snap.Message, "webdav e2e")
	}

	// Verify chunk content in destination.
	chunk, err := dstStore.GetChunk(context.Background(), chunkHash)
	if err != nil {
		t.Fatalf("GetChunk on dst: %v", err)
	}
	if string(chunk.Data) != "hello world chunk data" {
		t.Errorf("chunk data = %q, want %q", string(chunk.Data), "hello world chunk data")
	}

	// Suppress unused warning.
	_ = url.Parse
	_ = filepath.Join
	_ = core.Hash{}
}
