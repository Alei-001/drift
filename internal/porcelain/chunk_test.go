package porcelain

import (
	"context"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
	"github.com/zeebo/blake3"
)

// TestComputeFileHashFromChunks_Empty verifies that hashing zero chunks
// produces the BLAKE3 hash of the empty byte sequence. This is the baseline
// identity: hashing no chunk hashes yields the same result as hashing "".
func TestComputeFileHashFromChunks_Empty(t *testing.T) {
	got := computeFileHashFromChunks(nil)
	var want core.Hash
	h := blake3.New()
	copy(want[:], h.Sum(nil))
	if got != want {
		t.Errorf("expected %x, got %x", want, got)
	}
}

// TestComputeFileHashFromChunks_Deterministic verifies that the same chunk
// list always yields the same file hash.
func TestComputeFileHashFromChunks_Deterministic(t *testing.T) {
	chunks := []*core.Chunk{
		{Hash: core.Hash{0x01}},
		{Hash: core.Hash{0x02}},
		{Hash: core.Hash{0x03}},
	}
	a := computeFileHashFromChunks(chunks)
	b := computeFileHashFromChunks(chunks)
	if a != b {
		t.Errorf("expected deterministic hash, got %x and %x", a, b)
	}
}

// TestComputeFileHashFromChunks_OrderMatters verifies that the chunk hash
// order affects the file hash. The file hash is computed by hashing the
// concatenation of chunk hashes in order, so reordering chunks must produce
// a different file hash.
func TestComputeFileHashFromChunks_OrderMatters(t *testing.T) {
	c1 := []*core.Chunk{{Hash: core.Hash{0x01}}, {Hash: core.Hash{0x02}}}
	c2 := []*core.Chunk{{Hash: core.Hash{0x02}}, {Hash: core.Hash{0x01}}}
	if computeFileHashFromChunks(c1) == computeFileHashFromChunks(c2) {
		t.Error("expected different hashes for different chunk order")
	}
}

// TestComputeFileHashFromChunks_MatchesConcat verifies the hash matches the
// BLAKE3 of the concatenated chunk-hash bytes. This pins the wire format so
// accidental changes to the hashing scheme are caught.
func TestComputeFileHashFromChunks_MatchesConcat(t *testing.T) {
	chunks := []*core.Chunk{
		{Hash: core.Hash{0xAB}},
		{Hash: core.Hash{0xCD}},
	}
	got := computeFileHashFromChunks(chunks)

	h := blake3.New()
	h.Write(chunks[0].Hash[:])
	h.Write(chunks[1].Hash[:])
	var want core.Hash
	copy(want[:], h.Sum(nil))
	if got != want {
		t.Errorf("expected %x, got %x", want, got)
	}
}

// TestCountFileLines verifies that newlines are counted correctly across
// multiple chunks without concatenating them. This is a regression test for
// OOM: the old code appended all chunk data into a single []byte before
// counting, which would OOM on large files (e.g. 200 MB text).
func TestCountFileLines(t *testing.T) {
	hash1 := core.Hash{0x01}
	hash2 := core.Hash{0x02}
	hash3 := core.Hash{0x03}

	store := memory.NewMemoryStorage()
	store.PutChunk(context.Background(), &core.Chunk{Hash: hash1, Data: []byte("line1\nline2\n")})        // 2 newlines
	store.PutChunk(context.Background(), &core.Chunk{Hash: hash2, Data: []byte("line3\nline4")})          // 1 newline
	store.PutChunk(context.Background(), &core.Chunk{Hash: hash3, Data: []byte("line5\nline6\nline7\n")}) // 3 newlines

	entry := core.FileEntry{
		Chunks: []core.Hash{hash1, hash2, hash3},
	}

	count := CountFileLines(context.Background(), store, entry)
	if count != 6 {
		t.Errorf("expected 6 newlines, got %d", count)
	}
}

// TestCountFileLines_MissingChunk verifies that a missing chunk causes
// CountFileLines to return 0 rather than panicking.
func TestCountFileLines_MissingChunk(t *testing.T) {
	store := memory.NewMemoryStorage()
	entry := core.FileEntry{
		Chunks: []core.Hash{{0x01}},
	}
	count := CountFileLines(context.Background(), store, entry)
	if count != 0 {
		t.Errorf("expected 0 for missing chunk, got %d", count)
	}
}
