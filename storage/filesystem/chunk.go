package filesystem

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/util/fsutil"
	"github.com/zeebo/blake3"
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

	// Verify data integrity: hash should match
	computedHash := core.Hash(blake3.Sum256(data))
	if computedHash != hash {
		return nil, fmt.Errorf("chunk data integrity check failed: expected %s, got %s", hash.FullString(), computedHash.FullString())
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
	if err := fsutil.WriteFileAtomic(path, compressed, 0644); err != nil {
		return err
	}

	// Update cache after successful write
	fs.chunkCache.Add(chunk.Hash, chunk)
	return nil
}

// DeleteChunk removes a chunk from disk and the in-memory cache. It is
// idempotent: a missing file is not an error.
func (fs *FSStorage) DeleteChunk(hash core.Hash) error {
	path := fs.chunkPath(hash)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fs.chunkCache.Remove(hash)
	return nil
}

// ListChunks returns the hashes of all chunks stored on disk. The order
// of the returned slice is not guaranteed.
func (fs *FSStorage) ListChunks() ([]core.Hash, error) {
	chunksDir := fs.chunksDir()
	var hashes []core.Hash
	err := filepath.WalkDir(chunksDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(chunksDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		if len(parts) != 2 {
			return nil
		}
		b, err := hex.DecodeString(parts[0] + parts[1])
		if err != nil {
			return err
		}
		var h core.Hash
		copy(h[:], b)
		hashes = append(hashes, h)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return hashes, nil
		}
		return nil, err
	}
	return hashes, nil
}
