package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/klauspost/compress/zstd"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/util/cache"
)

type FSStorage struct {
	root         string
	chunkCache   *cache.Cache[core.Hash, *core.Chunk]
	previewCache *cache.Cache[string, []byte]
	// lifecycleMu guards the lifecycle transitions of the zstd
	// encoder/decoder (SetCompression and Close). It does NOT protect
	// data-access methods (GetChunk/PutChunk): those rely on the
	// porcelain workspace lock guaranteeing single-threaded access.
	lifecycleMu  sync.Mutex
	zstdDecoder  *zstd.Decoder
	zstdEncoder  *zstd.Encoder
	compression  bool
}

func NewFSStorage(root string) (*FSStorage, error) {
	dirs := []string{
		filepath.Join(root, ChunksDir),
		filepath.Join(root, SnapshotsDir),
		filepath.Join(root, ManifestsDir),
		filepath.Join(root, RefsDir),
		filepath.Join(root, PreviewsDir),
		filepath.Join(root, RefsDir, HeadsDir),
		filepath.Join(root, RefsDir, TagsDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", d, err)
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
	fs.lifecycleMu.Lock()
	defer fs.lifecycleMu.Unlock()
	fs.zstdDecoder.Close()
	if err := fs.zstdEncoder.Close(); err != nil {
		return fmt.Errorf("close zstd encoder: %w", err)
	}
	return nil
}

func (fs *FSStorage) SetCompression(enabled bool, level zstd.EncoderLevel) error {
	fs.lifecycleMu.Lock()
	defer fs.lifecycleMu.Unlock()
	fs.compression = enabled
	if enabled {
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(level))
		if err != nil {
			return fmt.Errorf("create zstd encoder: %w", err)
		}
		fs.zstdEncoder.Close()
		fs.zstdEncoder = enc
	}
	return nil
}
