package chunker

import (
	"fmt"
	"io"

	"github.com/your-org/drift/core"
	"github.com/zeebo/blake3"
)

type FixedChunker struct {
	chunkSize int
}

func NewFixedChunker(chunkSize int) *FixedChunker {
	if chunkSize < 4096 {
		chunkSize = 4096
	}
	if chunkSize > 64*1024*1024 { // 64MB upper limit
		chunkSize = 64 * 1024 * 1024
	}
	return &FixedChunker{chunkSize: chunkSize}
}

func (f *FixedChunker) Chunk(r io.Reader) ([]*core.Chunk, error) {
	buf := make([]byte, f.chunkSize)
	var chunks []*core.Chunk

	for {
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
			chunks = append(chunks, chunk)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("fixed chunker: read error: %w", err)
		}
	}

	return chunks, nil
}
