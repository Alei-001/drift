package porcelain

import (
	"context"
	"fmt"
	"io"
	"math"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/zeebo/blake3"
)

// wholeFileChunkThreshold is the maximum file size that chunkFile will read
// as a single chunk on the nil-chunker path. It matches TextEngine's
// whole-file threshold: files larger than this require a real chunker to
// avoid buffering the entire file in memory.
const wholeFileChunkThreshold = 64 * 1024

// chunkFile chunks a file using the engine-selected chunker. If the engine
// returns a nil chunker (or the file is empty), the whole file is read as a
// single chunk. Large files (>64 KB) are rejected on the nil-chunker path to
// avoid OOM; engines that return nil are expected to do so only for small
// files (see TextEngine.ChunkerFor).
func chunkFile(ctx context.Context, path string, r io.Reader, engine filetype.Engine, fileSize int64, cfg *core.CoreConfig) ([]*core.Chunk, error) {
	c := engine.ChunkerFor(fileSize, cfg)
	if fileSize == 0 {
		c = nil
	}
	if c == nil {
		// Reject large files before reading them into memory. The
		// nil-chunker path reads the whole file as a single chunk, so
		// a 500 MB video would OOM. 64 KB matches TextEngine's
		// whole-file threshold.
		if fileSize > wholeFileChunkThreshold {
			return nil, fmt.Errorf("file %s too large (%d bytes) for whole-file chunking without chunker", path, fileSize)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		// core.Chunk.Size is uint32; reject files whose single-chunk
		// representation would overflow it. In practice this path is
		// only reached for small text files (< 64KB), but guard
		// defensively in case a future engine returns nil for a
		// large file.
		if uint64(len(data)) > math.MaxUint32 {
			return nil, fmt.Errorf("file too large for single-chunk storage (%d bytes)", len(data))
		}
		sum := blake3.Sum256(data)
		var hash core.Hash
		copy(hash[:], sum[:])
		chunk := &core.Chunk{
			Hash:  hash,
			Size:  uint32(len(data)),
			Data:  data,
			Flags: core.ChunkFlagNone,
		}
		return []*core.Chunk{chunk}, nil
	}
	return c.Chunk(ctx, r)
}

// computeFileHashFromChunks derives the file-level hash by hashing the
// concatenation of chunk hashes. This makes the file hash independent of
// chunk data layout details and lets CreateSnapshot and ComputeFileHash
// produce identical hashes for the same file.
func computeFileHashFromChunks(chunks []*core.Chunk) core.Hash {
	fileHasher := blake3.New()
	for _, c := range chunks {
		fileHasher.Write(c.Hash[:])
	}
	var fileHash core.Hash
	copy(fileHash[:], fileHasher.Sum(nil))
	return fileHash
}
