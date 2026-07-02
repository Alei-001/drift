package stream

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/your-org/drift/core"
	"github.com/zeebo/blake3"
)

type ChunkGetter interface {
	GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error)
}

type chunkReader struct {
	ctx    context.Context
	store  ChunkGetter
	hashes []core.Hash
	idx    int
	cur    io.Reader
	err    error
}

func NewChunkReader(ctx context.Context, store ChunkGetter, hashes []core.Hash) io.Reader {
	return &chunkReader{ctx: ctx, store: store, hashes: hashes}
}

func (cr *chunkReader) Read(p []byte) (int, error) {
	if cr.err != nil {
		return 0, cr.err
	}
	for {
		if cr.cur == nil {
			if cr.idx >= len(cr.hashes) {
				return 0, io.EOF
			}
			hash := cr.hashes[cr.idx]
			chunk, err := cr.store.GetChunk(cr.ctx, hash)
			if err != nil {
				cr.err = fmt.Errorf("read chunk %s: %w", hash.String(), err)
				return 0, cr.err
			}
			cr.cur = bytes.NewReader(chunk.Data)
			cr.idx++
		}
		n, err := cr.cur.Read(p)
		if err == io.EOF {
			cr.cur = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func PeekHeader(r io.Reader, n int) (header []byte, rest io.Reader, err error) {
	buf := make([]byte, n)
	got, err := io.ReadFull(r, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, nil, err
	}
	header = buf[:got]
	return header, io.MultiReader(bytes.NewReader(header), r), nil
}

func HashFileContent(path string) (core.Hash, error) {
	f, err := os.Open(path)
	if err != nil {
		return core.Hash{}, err
	}
	defer f.Close()
	h := blake3.New()
	if _, err := io.Copy(h, f); err != nil {
		return core.Hash{}, err
	}
	var out core.Hash
	copy(out[:], h.Sum(nil))
	return out, nil
}

func HashChunkData(ctx context.Context, store ChunkGetter, hashes []core.Hash) (core.Hash, error) {
	h := blake3.New()
	for _, ch := range hashes {
		chunk, err := store.GetChunk(ctx, ch)
		if err != nil {
			return core.Hash{}, fmt.Errorf("read chunk %s: %w", ch.String(), err)
		}
		h.Write(chunk.Data)
	}
	var out core.Hash
	copy(out[:], h.Sum(nil))
	return out, nil
}
