package filesystem

import (
	"bytes"
	"os"
	"testing"

	"github.com/your-org/drift/core"
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
	if err := fs.PutChunk(chunk); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	// Retrieve the chunk — should succeed and match the original data.
	retrieved, err := fs.GetChunk(hash)
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
	_, err = fs.GetChunk(hash)
	if err == nil {
		t.Fatal("expected error for corrupted chunk, got nil")
	}
}
