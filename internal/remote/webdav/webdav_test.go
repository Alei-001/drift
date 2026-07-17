package webdav

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/memory"
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

// newWebDAVTestFS creates a fresh WebDAVFS connected to a local test
// server. It is the factory passed to RunRemoteFSConformance.
func newWebDAVTestFS(t *testing.T) (RemoteFS, func()) {
	t.Helper()
	srvURL, srvCleanup := startWebDAVTestServer(t)
	cfg := RemoteConfig{
		Name: "test",
		Type: "webdav",
		URL:  srvURL,
		User: "test",
		Options: map[string]string{
			"_password": "secret",
		},
		// httptest.NewServer returns http://127.0.0.1:port; the test
		// server is local so AllowInsecure is safe.
		AllowInsecure: true,
	}
	rfs, err := NewWebDAVFS(cfg)
	if err != nil {
		srvCleanup()
		t.Fatalf("NewWebDAVFS: %v", err)
	}
	return rfs, func() {
		rfs.Close()
		srvCleanup()
	}
}

// TestWebDAVConformance runs the shared RemoteFS conformance suite against
// a real local WebDAV server (golang.org/x/net/webdav + httptest).
func TestWebDAVConformance(t *testing.T) {
	RunRemoteFSConformance(t, "webdav", newWebDAVTestFS)
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
		// httptest.NewServer returns http://127.0.0.1:port; the test
		// server is local so AllowInsecure is safe.
		AllowInsecure: true,
	}

	// Source store: has a snapshot + chunk.
	srcStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer srcStore.Close()
	snapID, chunkHash := makeTestSnapshot(t, srcStore, "webdav e2e", nil)
	setupBranchRef(t, srcStore, "main", snapID.Hash)

	// Push from source to WebDAV remote.
	rfsPush, err := NewWebDAVFS(cfg)
	if err != nil {
		t.Fatalf("NewWebDAVFS (push): %v", err)
	}
	defer rfsPush.Close()

	stats, err := Push(context.Background(), srcStore, rfsPush, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if stats.SnapshotsUploaded != 1 || stats.ChunksUploaded != 1 {
		t.Fatalf("expected 1 snap + 1 chunk uploaded, got snap=%d chunk=%d",
			stats.SnapshotsUploaded, stats.ChunksUploaded)
	}

	// Pull to a fresh destination store.
	dstStore := store.NewStoreSet(memory.NewMemoryStorage())
	defer dstStore.Close()

	rfsPull, err := NewWebDAVFS(cfg)
	if err != nil {
		t.Fatalf("NewWebDAVFS (pull): %v", err)
	}
	defer rfsPull.Close()

	stats, err = Pull(context.Background(), dstStore, rfsPull, "", SyncOptions{})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if stats.SnapshotsUploaded != 1 || stats.ChunksUploaded != 1 {
		t.Fatalf("expected 1 snap + 1 chunk downloaded, got snap=%d chunk=%d",
			stats.SnapshotsUploaded, stats.ChunksUploaded)
	}

	// Verify snapshot content in destination.
	snap, err := dstStore.Snapshots.GetSnapshot(context.Background(), snapID)
	if err != nil {
		t.Fatalf("GetSnapshot on dst: %v", err)
	}
	if snap.Message != "webdav e2e" {
		t.Errorf("snap.Message = %q, want %q", snap.Message, "webdav e2e")
	}

	// Verify chunk content in destination.
	chunk, err := dstStore.Chunks.GetChunk(context.Background(), chunkHash)
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
