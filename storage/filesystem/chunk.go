package filesystem

import (
	"os"
	"path/filepath"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/util/fsutil"
)

func (fs *FSStorage) chunksDir() string {
	return filepath.Join(fs.root, ChunksDir)
}

// chunkPath returns the filesystem path for a chunk, using the schema:
//
//	chunks/{hex[0:2]}/{hex[2:]}
func (fs *FSStorage) chunkPath(hash core.Hash) string {
	hex := hash.FullString()
	return filepath.Join(fs.chunksDir(), hex[:2], hex[2:])
}

// HasChunk returns true if the chunk exists on disk.
func (fs *FSStorage) HasChunk(hash core.Hash) bool {
	path := fs.chunkPath(hash)
	_, err := os.Stat(path)
	return err == nil
}

// GetChunk reads a chunk from disk, returning the decompressed data.
func (fs *FSStorage) GetChunk(hash core.Hash) (*core.Chunk, error) {
	if ch, ok := fs.chunkCache.Get(hash); ok {
		return ch, nil
	}

	path := fs.chunkPath(hash)
	compressed, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	data, err := fs.zstdDecoder.DecodeAll(compressed, nil)
	if err != nil {
		return nil, err
	}

	ch := &core.Chunk{
		Hash:  hash,
		Size:  uint32(len(data)),
		Data:  data,
		Flags: 0,
	}
	fs.chunkCache.Add(hash, ch)
	return ch, nil
}

// PutChunk compresses and writes a chunk to disk.
func (fs *FSStorage) PutChunk(chunk *core.Chunk) error {
	if fs.HasChunk(chunk.Hash) {
		return nil
	}

	path := fs.chunkPath(chunk.Hash)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	compressed := fs.zstdEncoder.EncodeAll(chunk.Data, nil)
	return fsutil.WriteFileAtomic(path, compressed, 0644)
}
