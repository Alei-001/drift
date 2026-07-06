package porcelain

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/your-org/drift/internal/core"
)

// TestCheckWithin_Inside verifies that a path inside baseDir is accepted.
func TestCheckWithin_Inside(t *testing.T) {
	base := filepath.Join(os.TempDir(), "ws")
	inside := filepath.Join(base, "sub", "file.txt")
	if err := checkWithin(base, inside); err != nil {
		t.Errorf("expected nil for path inside base, got %v", err)
	}
}

// TestCheckWithin_EscapesRoot verifies that a path that escapes baseDir via
// ".." is rejected.
func TestCheckWithin_EscapesRoot(t *testing.T) {
	base := filepath.Join(os.TempDir(), "ws")
	outside := filepath.Join(base, "..", "evil.txt")
	if err := checkWithin(base, outside); err == nil {
		t.Error("expected error for path escaping base, got nil")
	}
}

// TestCheckWithin_UnrelatedPath verifies that a completely unrelated path is
// rejected.
func TestCheckWithin_UnrelatedPath(t *testing.T) {
	base := filepath.Join(os.TempDir(), "ws")
	other := filepath.Join(os.TempDir(), "other", "file.txt")
	if err := checkWithin(base, other); err == nil {
		t.Error("expected error for unrelated path, got nil")
	}
}

// TestCheckWithin_BaseItself verifies that passing baseDir itself as the
// target is accepted (rel == ".").
func TestCheckWithin_BaseItself(t *testing.T) {
	base := filepath.Join(os.TempDir(), "ws")
	if err := checkWithin(base, base); err != nil {
		t.Errorf("expected nil for base itself, got %v", err)
	}
}

// TestResolveSecurePath_InsideWorkspace verifies that a regular relative path
// inside an existing workspace is accepted and returns the joined absolute
// path.
func TestResolveSecurePath_InsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	rel := "sub/file.txt"
	got, err := resolveSecurePath(dir, rel)
	if err != nil {
		t.Fatalf("resolveSecurePath failed: %v", err)
	}
	want := filepath.Join(dir, rel)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestResolveSecurePath_RelativeWithDotDot verifies that a relative path
// containing ".." that would escape the workspace is rejected. The path is
// rejected because filepath.Rel reports it as starting with "..".
func TestResolveSecurePath_RelativeWithDotDot(t *testing.T) {
	dir := t.TempDir()
	// Create a real subdir so EvalSymlinks does not walk up looking for
	// an existing ancestor and accidentally succeeding.
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// "../evil.txt" joined with workDir would escape it.
	rel := filepath.Join("sub", "..", "..", "evil.txt")
	_, err := resolveSecurePath(dir, rel)
	if err == nil {
		t.Error("expected error for path escaping workspace, got nil")
	}
}

// TestResolveSecurePath_NonExistentWorkDir verifies that a non-existent
// workspace directory yields an error (EvalSymlinks fails on the workDir
// itself).
func TestResolveSecurePath_NonExistentWorkDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := resolveSecurePath(dir, "file.txt")
	if err == nil {
		t.Error("expected error for non-existent workDir, got nil")
	}
}

// TestWriteFileFromChunks_RoundTrip verifies that writeFileFromChunks
// reconstructs a file by concatenating chunk data in order, then renames the
// temp file into place atomically.
func TestWriteFileFromChunks_RoundTrip(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	ctx := context.Background()

	chunkA := &core.Chunk{Hash: core.Hash{0xA1}, Data: []byte("Hello, ")}
	chunkB := &core.Chunk{Hash: core.Hash{0xA2}, Data: []byte("World!")}
	store.PutChunk(ctx, chunkA)
	store.PutChunk(ctx, chunkB)

	dst := filepath.Join(dir, "out.txt")
	if err := writeFileFromChunks(ctx, store, dst, []core.Hash{chunkA.Hash, chunkB.Hash}, 0644); err != nil {
		t.Fatalf("writeFileFromChunks failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", string(got))
	}

	// No .drifttmp file should be left behind after a successful write.
	tmp := dst + ".drifttmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("expected no .drifttmp file left behind, got err=%v", err)
	}
}

// TestWriteFileFromChunks_MissingChunkCleansUp verifies that when a chunk is
// missing, the temp file is removed and the destination is left untouched.
func TestWriteFileFromChunks_MissingChunkCleansUp(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	ctx := context.Background()

	chunkA := &core.Chunk{Hash: core.Hash{0xB1}, Data: []byte("partial ")}
	store.PutChunk(ctx, chunkA)
	missingHash := core.Hash{0xB2} // never stored

	dst := filepath.Join(dir, "out.txt")
	err := writeFileFromChunks(ctx, store, dst, []core.Hash{chunkA.Hash, missingHash}, 0644)
	if err == nil {
		t.Fatal("expected error for missing chunk, got nil")
	}
	if !strings.Contains(err.Error(), "get chunk") {
		t.Errorf("expected 'get chunk' in error, got %q", err.Error())
	}

	// Destination must not exist; temp file must be cleaned up.
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("expected destination to not exist after failure, got err=%v", err)
	}
	tmp := dst + ".drifttmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("expected temp file cleaned up after failure, got err=%v", err)
	}
}

// TestWriteFileFromChunks_EmptyChunkList verifies that writing with no chunks
// produces an empty file. This is the empty-file restore path.
func TestWriteFileFromChunks_EmptyChunkList(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	ctx := context.Background()

	dst := filepath.Join(dir, "empty.txt")
	if err := writeFileFromChunks(ctx, store, dst, nil, 0644); err != nil {
		t.Fatalf("writeFileFromChunks with no chunks failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(got))
	}
}
