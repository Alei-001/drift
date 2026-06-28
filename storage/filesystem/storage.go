package filesystem

import (
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/util/cache"
)

// FSStorage implements storage.Storer using the local filesystem.
type FSStorage struct {
	root         string
	chunkCache   *cache.Cache[core.Hash, *core.Chunk]
	previewCache *cache.Cache[string, []byte]
	zstdDecoder  *zstd.Decoder
	zstdEncoder  *zstd.Encoder
}

// NewFSStorage creates or opens a filesystem-backed storage at root (the .drift/ directory).
func NewFSStorage(root string) (*FSStorage, error) {
	dirs := []string{
		filepath.Join(root, ChunksDir),
		filepath.Join(root, SnapshotsDir),
		filepath.Join(root, RefsDir),
		filepath.Join(root, PreviewsDir),
		filepath.Join(root, RefsDir, HeadsDir),
		filepath.Join(root, RefsDir, TagsDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, err
		}
	}

	chunkCache, err := cache.NewCache[core.Hash, *core.Chunk](256)
	if err != nil {
		return nil, err
	}
	previewCache, err := cache.NewCache[string, []byte](64)
	if err != nil {
		return nil, err
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, err
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		decoder.Close()
		return nil, err
	}

	return &FSStorage{
		root:         root,
		chunkCache:   chunkCache,
		previewCache: previewCache,
		zstdDecoder:  decoder,
		zstdEncoder:  encoder,
	}, nil
}

// Close releases resources held by the storage.
func (fs *FSStorage) Close() error {
	fs.zstdDecoder.Close()
	return fs.zstdEncoder.Close()
}
