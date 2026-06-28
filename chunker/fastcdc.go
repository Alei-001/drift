package chunker

import (
	"bytes"
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
	// Peek first byte to check if stream is empty
	var buf [1]byte
	_, peekErr := r.Read(buf[:])
	if peekErr == io.EOF {
		return nil, nil
	}
	if peekErr != nil {
		return nil, peekErr
	}
	// Reconstruct reader with peeked byte
	r = io.MultiReader(bytes.NewReader(buf[:]), r)

	ch, err := cdc.NewChunker("fastcdc", r, &cdc.ChunkerOpts{
		MinSize:    fastCDCMinSize,
		NormalSize: fastCDCAvgSize,
		MaxSize:    fastCDCMaxSize,
	})
	if err != nil {
		return nil, err
	}

	var chunks []*core.Chunk

	err = ch.Split(func(offset, length uint, chunkData []byte) error {
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
