package chunker

import (
	"io"

	cdc "github.com/PlakarKorp/go-cdc-chunkers"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/fastcdc"
	"github.com/your-org/drift/core"
	"github.com/zeebo/blake3"
)

const (
	fastCDCMinSize = 131072 // 128KB
	fastCDCAvgSize = 262144 // 256KB
	fastCDCMaxSize = 524288 // 512KB
)

type FastCDCChunker struct{}

func NewFastCDCChunker() *FastCDCChunker {
	return &FastCDCChunker{}
}

func (f *FastCDCChunker) Chunk(r io.Reader) ([]*core.Chunk, error) {
	// Use "fastcdc-v1.0.0" instead of legacy "fastcdc": the legacy mode
	// forces hardcoded masks computed for an 8KB NormalSize, which would
	// skew cut points for our 128KB/256KB/512KB sizes. The v1.0.0 variant
	// computes masks dynamically from the actual NormalSize.
	ch, err := cdc.NewChunker("fastcdc-v1.0.0", r, &cdc.ChunkerOpts{
		MinSize:    fastCDCMinSize,
		NormalSize: fastCDCAvgSize,
		MaxSize:    fastCDCMaxSize,
	})
	if err != nil {
		return nil, err
	}

	var chunks []*core.Chunk

	err = ch.Split(func(offset, length uint, chunkData []byte) error {
		// The chunker may emit a zero-length chunk (e.g. for an empty
		// stream); skip it so callers never see empty chunks.
		if length == 0 {
			return nil
		}
		var hash core.Hash
		sum := blake3.Sum256(chunkData)
		copy(hash[:], sum[:])

		chunk := &core.Chunk{
			Hash:  hash,
			Size:  uint32(length),
			Data:  make([]byte, length),
			Flags: core.ChunkFlagNone,
		}
		copy(chunk.Data, chunkData)

		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return chunks, nil
}
