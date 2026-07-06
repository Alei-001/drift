package filesystem

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/zeebo/blake3"
)

// TestGetChunk_IntegrityCheck verifies that GetChunk detects on-disk
// corruption of a chunk. The chunk is first stored and read back
// successfully; the file is then overwritten on disk and the cache is
// invalidated so the next GetChunk is forced to read from disk.
func TestGetChunk_IntegrityCheck(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	// Create a chunk with known data.
	data := []byte("test chunk data")
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])

	chunk := &core.Chunk{
		Hash: hash,
		Size: uint32(len(data)),
		Data: data,
	}

	// Store the chunk.
	if err := fs.PutChunk(context.Background(), chunk); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	// Retrieve the chunk — should succeed and match the original data.
	retrieved, err := fs.GetChunk(context.Background(), hash)
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if !bytes.Equal(retrieved.Data, data) {
		t.Fatalf("retrieved data mismatch: got %q, want %q", retrieved.Data, data)
	}

	// Remove the chunk from the in-memory cache so the next read comes
	// from disk.
	fs.chunkCache.Remove(hash)

	// Corrupt the chunk file on disk. We write valid zstd-compressed data
	// of DIFFERENT content so that decompression succeeds but the blake3
	// hash no longer matches, directly exercising the integrity check.
	chunkPath := fs.chunkPath(hash)
	fakeCompressed := fs.zstdEncoder.EncodeAll([]byte("totally different content"), nil)
	if err := os.WriteFile(chunkPath, fakeCompressed, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// GetChunk must now report an error from the integrity check.
	_, err = fs.GetChunk(context.Background(), hash)
	if err == nil {
		t.Fatal("expected error for corrupted chunk, got nil")
	}
}

// TestDeleteChunk_Idempotent verifies that deleting a chunk that does not
// exist returns nil (no error).
func TestDeleteChunk_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	data := []byte("non-existent chunk")
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])

	// Deleting a chunk that was never stored must not error.
	if err := fs.DeleteChunk(context.Background(), hash); err != nil {
		t.Fatalf("DeleteChunk on non-existent chunk: expected nil, got %v", err)
	}
}

// TestDeleteChunk_ClearsCache verifies that DeleteChunk removes the cache
// entry so a subsequent GetChunk does not return stale cached data.
func TestDeleteChunk_ClearsCache(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	data := []byte("cache clear test data")
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])

	chunk := &core.Chunk{
		Hash: hash,
		Size: uint32(len(data)),
		Data: data,
	}

	if err := fs.PutChunk(context.Background(), chunk); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	// Read the chunk back so it enters the cache.
	retrieved, err := fs.GetChunk(context.Background(), hash)
	if err != nil {
		t.Fatalf("GetChunk before delete: %v", err)
	}
	if !bytes.Equal(retrieved.Data, data) {
		t.Fatalf("data mismatch before delete: got %q, want %q", retrieved.Data, data)
	}

	// Delete the chunk — this must clear both the file and the cache.
	if err := fs.DeleteChunk(context.Background(), hash); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}

	// GetChunk must now fail: the file is gone and the cache entry was
	// removed.
	if _, err := fs.GetChunk(context.Background(), hash); err == nil {
		t.Fatal("expected error after DeleteChunk (cache should be cleared), got nil")
	}
}

// TestListChunks verifies that ListChunks returns all stored chunk hashes
// and returns an empty result for an empty store.
func TestListChunks(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	// Empty storage: expect an empty slice with no error.
	hashes, err := fs.ListChunks(context.Background())
	if err != nil {
		t.Fatalf("ListChunks on empty storage: %v", err)
	}
	if len(hashes) != 0 {
		t.Fatalf("expected 0 chunks on empty storage, got %d", len(hashes))
	}

	// Store two distinct chunks.
	data1 := []byte("list chunks test one")
	data2 := []byte("list chunks test two")
	var hash1, hash2 core.Hash
	sum1 := blake3.Sum256(data1)
	sum2 := blake3.Sum256(data2)
	copy(hash1[:], sum1[:])
	copy(hash2[:], sum2[:])

	chunk1 := &core.Chunk{Hash: hash1, Size: uint32(len(data1)), Data: data1}
	chunk2 := &core.Chunk{Hash: hash2, Size: uint32(len(data2)), Data: data2}

	if err := fs.PutChunk(context.Background(), chunk1); err != nil {
		t.Fatalf("PutChunk 1: %v", err)
	}
	if err := fs.PutChunk(context.Background(), chunk2); err != nil {
		t.Fatalf("PutChunk 2: %v", err)
	}

	hashes, err = fs.ListChunks(context.Background())
	if err != nil {
		t.Fatalf("ListChunks: %v", err)
	}
	if len(hashes) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(hashes))
	}

	// Verify both hashes are present (order is not guaranteed).
	want := map[core.Hash]bool{hash1: true, hash2: true}
	for _, h := range hashes {
		if !want[h] {
			t.Fatalf("unexpected hash %s in ListChunks result", h.FullString())
		}
	}
}

// TestListChunks_SkipsInvalidFiles verifies that non-chunk files in the
// chunks directory (e.g. .DS_Store) are skipped instead of aborting.
func TestListChunks_SkipsInvalidFiles(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	// Store one real chunk.
	data := []byte("real chunk data")
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])
	if err := fs.PutChunk(context.Background(), &core.Chunk{Hash: hash, Size: uint32(len(data)), Data: data}); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	// Drop the cache so ListChunks is forced to scan disk (not strictly
	// necessary for ListChunks, but mirrors real-world conditions).
	fs.chunkCache.Remove(hash)

	// Place a stray non-hex file in a chunk subdirectory.
	subDir := filepath.Join(fs.chunksDir(), hash.FullString()[:2])
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, ".DS_Store"), []byte("junk"), 0644); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	hashes, err := fs.ListChunks(context.Background())
	if err != nil {
		t.Fatalf("ListChunks should skip invalid files, got error: %v", err)
	}
	if len(hashes) != 1 || hashes[0] != hash {
		t.Fatalf("expected only the real chunk hash, got %v", hashes)
	}
}

// TestGetChunk_LargeCompressed verifies that a large chunk stored with
// compression is read back correctly via the streaming zstd decode path
// (zstdDecoder.Reset + io.ReadAll), without panic or data corruption.
func TestGetChunk_LargeCompressed(t *testing.T) {
	tmpDir := t.TempDir()

	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	// ~1.3 MB of highly compressible data: exercises the streaming decode
	// path while keeping the test fast.
	data := bytes.Repeat([]byte("drift-streaming-chunk-test-"), 50_000)
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])

	chunk := &core.Chunk{Hash: hash, Size: uint32(len(data)), Data: data}
	if err := fs.PutChunk(context.Background(), chunk); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	// Evict the in-memory cache so GetChunk must read from disk, exercising
	// the streaming zstd decode path.
	fs.chunkCache.Remove(hash)

	retrieved, err := fs.GetChunk(context.Background(), hash)
	if err != nil {
		t.Fatalf("GetChunk large compressed: %v", err)
	}
	if len(retrieved.Data) != len(data) {
		t.Fatalf("data length mismatch: got %d, want %d", len(retrieved.Data), len(data))
	}
	if !bytes.Equal(retrieved.Data, data) {
		t.Fatalf("large chunk data mismatch")
	}
	if retrieved.Flags != core.ChunkFlagCompressed {
		t.Errorf("expected ChunkFlagCompressed, got %v", retrieved.Flags)
	}

	// Verify the on-disk file actually starts with the compression flag so
	// we know the streaming decode path (not the uncompressed path) was
	// exercised.
	path := fs.chunkPath(hash)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open chunk file: %v", err)
	}
	header := make([]byte, 1)
	if _, err := f.Read(header); err != nil {
		t.Fatalf("read header: %v", err)
	}
	f.Close()
	if header[0]&chunkFlagCompressed == 0 {
		t.Fatal("on-disk chunk should have compression flag set")
	}
}
