package snapshot

import (
	"context"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/memory"
	"github.com/zeebo/blake3"
)

// TestComputeFileHashFromHashes_Empty verifies that hashing zero chunk
// hashes produces the BLAKE3 hash of the empty byte sequence. This is the
// baseline identity: hashing no chunk hashes yields the same result as
// hashing "".
func TestComputeFileHashFromHashes_Empty(t *testing.T) {
	got := computeFileHashFromHashes(nil)
	var want core.Hash
	h := blake3.New()
	copy(want[:], h.Sum(nil))
	if got != want {
		t.Errorf("expected %x, got %x", want, got)
	}
}

// TestComputeFileHashFromHashes_Deterministic verifies that the same chunk
// hash list always yields the same file hash.
func TestComputeFileHashFromHashes_Deterministic(t *testing.T) {
	hashes := []core.Hash{
		{0x01},
		{0x02},
		{0x03},
	}
	a := computeFileHashFromHashes(hashes)
	b := computeFileHashFromHashes(hashes)
	if a != b {
		t.Errorf("expected deterministic hash, got %x and %x", a, b)
	}
}

// TestComputeFileHashFromHashes_OrderMatters verifies that the chunk hash
// order affects the file hash. The file hash is computed by hashing the
// concatenation of chunk hashes in order, so reordering must produce a
// different file hash.
func TestComputeFileHashFromHashes_OrderMatters(t *testing.T) {
	c1 := []core.Hash{{0x01}, {0x02}}
	c2 := []core.Hash{{0x02}, {0x01}}
	if computeFileHashFromHashes(c1) == computeFileHashFromHashes(c2) {
		t.Error("expected different hashes for different chunk order")
	}
}

// TestComputeFileHashFromHashes_MatchesConcat verifies the hash matches the
// BLAKE3 of the concatenated chunk-hash bytes. This pins the wire format so
// accidental changes to the hashing scheme are caught.
func TestComputeFileHashFromHashes_MatchesConcat(t *testing.T) {
	hashes := []core.Hash{
		{0xAB},
		{0xCD},
	}
	got := computeFileHashFromHashes(hashes)

	h := blake3.New()
	h.Write(hashes[0][:])
	h.Write(hashes[1][:])
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
	data1 := []byte("line1\nline2\n")
	data2 := []byte("line3\nline4")
	data3 := []byte("line5\nline6\nline7\n")

	var hash1, hash2, hash3 core.Hash
	sum1 := blake3.Sum256(data1)
	sum2 := blake3.Sum256(data2)
	sum3 := blake3.Sum256(data3)
	copy(hash1[:], sum1[:])
	copy(hash2[:], sum2[:])
	copy(hash3[:], sum3[:])

	store := store.NewStoreSet(memory.NewMemoryStorage())
	if err := store.Chunks.PutChunk(context.Background(), &core.Chunk{Hash: hash1, Data: data1}); err != nil {
		t.Fatalf("PutChunk 1: %v", err)
	}
	if err := store.Chunks.PutChunk(context.Background(), &core.Chunk{Hash: hash2, Data: data2}); err != nil {
		t.Fatalf("PutChunk 2: %v", err)
	}
	if err := store.Chunks.PutChunk(context.Background(), &core.Chunk{Hash: hash3, Data: data3}); err != nil {
		t.Fatalf("PutChunk 3: %v", err)
	}

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
	store := store.NewStoreSet(memory.NewMemoryStorage())
	entry := core.FileEntry{
		Chunks: []core.Hash{{0x01}},
	}
	count := CountFileLines(context.Background(), store, entry)
	if count != 0 {
		t.Errorf("expected 0 for missing chunk, got %d", count)
	}
}
