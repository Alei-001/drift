package chunker

import (
	"context"
	"fmt"
	"io"

	"github.com/Alei-001/drift/internal/core"
	"github.com/zeebo/blake3"
)

// fixedChunkMinSize and fixedChunkMaxSize bound the chunk size accepted by
// NewFixedChunker. Sizes outside this range are clamped to the nearest bound.
const (
	fixedChunkMinSize = 4096
	fixedChunkMaxSize = 64 * 1024 * 1024
)

// FixedChunker implements Chunker by splitting the input into fixed-size
// chunks. Cut points are independent of the data, so identical bytes at
// different offsets produce different chunks.
type FixedChunker struct {
	chunkSize int
}

// NewFixedChunker creates a FixedChunker with the given chunk size. Sizes
// below fixedChunkMinSize or above fixedChunkMaxSize are clamped to the
// nearest bound.
func NewFixedChunker(chunkSize int) *FixedChunker {
	if chunkSize < fixedChunkMinSize {
		chunkSize = fixedChunkMinSize
	}
	if chunkSize > fixedChunkMaxSize {
		chunkSize = fixedChunkMaxSize
	}
	return &FixedChunker{chunkSize: chunkSize}
}

// Chunk splits r into consecutive fixed-size chunks. The final chunk may be
// shorter than the chunk size if the input length is not a multiple of it.
// Each chunk is BLAKE3-hashed. fn is invoked for every emitted chunk; if fn
// returns an error Chunk stops and returns it. The context is checked before
// each read so a cancelled context aborts the chunking loop promptly.
func (f *FixedChunker) Chunk(ctx context.Context, r io.Reader, fn func(*core.Chunk) error) error {
	buf := make([]byte, f.chunkSize)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := io.ReadFull(r, buf)
		if n == 0 && err == io.EOF {
			break
		}
		if n > 0 {
			chunkData := make([]byte, n)
			copy(chunkData, buf[:n])

			var hash core.Hash
			sum := blake3.Sum256(chunkData)
			copy(hash[:], sum[:])

			chunk := &core.Chunk{
				Hash:  hash,
				Size:  uint32(n),
				Data:  chunkData,
				Flags: core.ChunkFlagNone,
			}
			if err := fn(chunk); err != nil {
				return err
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return fmt.Errorf("fixed chunker: read error: %w", err)
		}
	}

	return nil
}
