package porcelain

import (
	"testing"

	"github.com/your-org/drift/internal/core"
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
