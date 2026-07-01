package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/util/cache"
)

type FSStorage struct {
	root         string
	chunkCache   *cache.Cache[core.Hash, *core.Chunk]
	previewCache *cache.Cache[string, []byte]
	zstdMu       sync.Mutex
	zstdDecoder  *zstd.Decoder
	zstdEncoder  *zstd.Encoder
	compression  bool
}

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
		return nil, fmt.Errorf("create chunk cache: %w", err)
	}
	previewCache, err := cache.NewCache[string, []byte](64)
	if err != nil {
		return nil, fmt.Errorf("create preview cache: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("create zstd decoder: %w", err)
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		decoder.Close()
		return nil, fmt.Errorf("create zstd encoder: %w", err)
	}

	return &FSStorage{
		root:         root,
		chunkCache:   chunkCache,
		previewCache: previewCache,
		zstdDecoder:  decoder,
		zstdEncoder:  encoder,
		compression:  true,
	}, nil
}

func (fs *FSStorage) Close() error {
	fs.zstdDecoder.Close()
	return fs.zstdEncoder.Close()
}

func (fs *FSStorage) SetCompression(enabled bool, level zstd.EncoderLevel) {
	fs.zstdMu.Lock()
	defer fs.zstdMu.Unlock()
	fs.compression = enabled
	if enabled {
		if enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(level)); err == nil {
			fs.zstdEncoder.Close()
			fs.zstdEncoder = enc
		}
	}
}
