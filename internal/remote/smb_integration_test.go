package remote

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

// smbTestEnv holds the connection parameters for a real SMB server,
// populated from environment variables. Tests skip when DRIFT_SMB_TEST_URL
// is unset, so the suite runs only when a server is explicitly configured.
type smbTestEnv struct {
	URL      string
	User     string
	Password string
	Domain   string
}

// loadSMBTestEnv reads the DRIFT_SMB_TEST_* environment variables. Returns
// ok=false when DRIFT_SMB_TEST_URL is empty, signalling the caller to skip.
func loadSMBTestEnv() (smbTestEnv, bool) {
	url := os.Getenv("DRIFT_SMB_TEST_URL")
	if url == "" {
		return smbTestEnv{}, false
	}
	return smbTestEnv{
		URL:      url,
		User:     os.Getenv("DRIFT_SMB_TEST_USER"),
		Password: os.Getenv("DRIFT_SMB_TEST_PASSWORD"),
		Domain:   os.Getenv("DRIFT_SMB_TEST_DOMAIN"),
	}, true
}

// newSMBTestFS creates a fresh SMBFS connected to a real SMB server
// configured via environment variables. It is the factory passed to
// RunRemoteFSConformance. When no server is configured, the calling test
// is skipped.
func newSMBTestFS(t *testing.T) (RemoteFS, func()) {
	t.Helper()
	env, ok := loadSMBTestEnv()
	if !ok {
		t.Skip("set DRIFT_SMB_TEST_URL to run SMB integration tests")
	}
	cfg := RemoteConfig{
		Name: "test",
		Type: "smb",
		URL:  env.URL,
		User: env.User,
		Options: map[string]string{
			"_password": env.Password,
			"domain":    env.Domain,
		},
	}
	rfs, err := NewSMBFS(cfg)
	if err != nil {
		t.Fatalf("NewSMBFS: %v", err)
	}
	return rfs, func() {
		if err := rfs.Close(); err != nil {
			t.Errorf("close SMB: %v", err)
		}
	}
}

// cleanRemoteDriftData removes the top-level drift directories (chunks,
// snapshots, manifests, refs) from the remote. This makes integration
// tests self-contained and repeatable by clearing any leftover data from
// previous runs or manual CLI testing. It performs a best-effort recursive
// removal: subdirectory entries are listed and their contents deleted
// before the subdirectory itself is removed.
func cleanRemoteDriftData(t *testing.T, rfs RemoteFS) {
	t.Helper()
	ctx := context.Background()
	for _, dir := range []string{"chunks", "snapshots", "manifests", "refs"} {
		removeRemoteDirRecursive(ctx, t, rfs, dir)
	}
}

// removeRemoteDirRecursive deletes all files and subdirectories under
// dirPath on the remote. It is best-effort: errors on individual entries
// are ignored so that one stale file does not abort the cleanup.
func removeRemoteDirRecursive(ctx context.Context, t *testing.T, rfs RemoteFS, dirPath string) {
	t.Helper()
	entries, err := rfs.List(ctx, dirPath)
	if err != nil {
		return // directory does not exist — nothing to clean
	}
	for _, e := range entries {
		if e.IsDir {
			removeRemoteDirRecursive(ctx, t, rfs, e.Path)
		} else {
			_ = rfs.Remove(ctx, e.Path)
		}
	}
}

// TestSMBConformance runs the shared RemoteFS conformance suite against a
// real SMB server configured via DRIFT_SMB_TEST_* environment variables.
// This covers ListRoot (regression test for the SMBFS.resolve bug),
// BasicCRUD, NestedPaths, OverwriteFile, RemoveMissing, ListEmpty, and
// MkdirAll.
func TestSMBConformance(t *testing.T) {
	RunRemoteFSConformance(t, "smb", newSMBTestFS)
}

// TestSMB_PushPullEndToEnd verifies the full push/pull flow over a real
// SMB server (not the mock). This catches wire-format mismatches that
// mock-only tests miss, and validates the SMBFS.resolve fix under real
// concurrency (multiple chunks uploaded simultaneously).
func TestSMB_PushPullEndToEnd(t *testing.T) {
	env, ok := loadSMBTestEnv()
	if !ok {
		t.Skip("set DRIFT_SMB_TEST_URL to run SMB integration tests")
	}

	cfg := RemoteConfig{
		Name: "test",
		Type: "smb",
		URL:  env.URL,
		User: env.User,
		Options: map[string]string{
			"_password": env.Password,
			"domain":    env.Domain,
		},
	}

	// Clean any leftover drift data on the share so the test is
	// self-contained and repeatable.
	rfsClean, err := NewSMBFS(cfg)
	if err != nil {
		t.Fatalf("NewSMBFS (clean): %v", err)
	}
	cleanRemoteDriftData(t, rfsClean)
	rfsClean.Close()

	// Source store: has a snapshot + chunk.
	srcStore := memory.NewMemoryStorage()
	defer srcStore.Close()
	snapID, chunkHash := makeTestSnapshot(t, srcStore, "smb e2e", nil)
	setupBranchRef(t, srcStore, "main", snapID.Hash)

	// Push from source to SMB remote.
	rfsPush, err := NewSMBFS(cfg)
	if err != nil {
		t.Fatalf("NewSMBFS (push): %v", err)
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
	dstStore := memory.NewMemoryStorage()
	defer dstStore.Close()

	rfsPull, err := NewSMBFS(cfg)
	if err != nil {
		t.Fatalf("NewSMBFS (pull): %v", err)
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
	snap, err := dstStore.GetSnapshot(context.Background(), snapID)
	if err != nil {
		t.Fatalf("GetSnapshot on dst: %v", err)
	}
	if snap.Message != "smb e2e" {
		t.Errorf("snap.Message = %q, want %q", snap.Message, "smb e2e")
	}

	// Verify chunk content in destination.
	chunk, err := dstStore.GetChunk(context.Background(), chunkHash)
	if err != nil {
		t.Fatalf("GetChunk on dst: %v", err)
	}
	if !bytes.Equal(chunk.Data, []byte("hello world chunk data")) {
		t.Errorf("chunk data = %q, want %q", string(chunk.Data), "hello world chunk data")
	}
}

// TestSMB_WriteNoPartialResidue verifies that after a successful Write, no
// .partial file remains on the share. SMBFS.Write uses an atomic
// partial-then-rename strategy; a leftover .partial would indicate the
// rename step failed silently.
func TestSMB_WriteNoPartialResidue(t *testing.T) {
	rfs, cleanup := newSMBTestFS(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	p := "no-residue-test.txt"

	if err := rfs.Write(ctx, p, strings.NewReader("no partial left")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The .partial file must not exist.
	if _, err := rfs.Stat(ctx, p+".partial"); err == nil {
		t.Error(".partial file remains after successful Write")
	} else if !strings.Contains(err.Error(), "not exist") {
		t.Errorf("unexpected error checking .partial: %v", err)
	}

	// The final file must exist with correct content.
	rc, err := rfs.Read(ctx, p)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "no partial left" {
		t.Errorf("Read = %q, want %q", string(data), "no partial left")
	}

	// Cleanup.
	_ = rfs.Remove(ctx, p)
}
