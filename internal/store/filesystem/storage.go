package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/klauspost/compress/zstd"
)

// Layout constants are re-exported from the storage interface package so
// existing references within the filesystem backend continue to work.
const (
	DriftDir     = store.DriftDir
	ChunksDir    = store.ChunksDir
	SnapshotsDir = store.SnapshotsDir
	ManifestsDir = store.ManifestsDir
	RefsDir      = store.RefsDir
	PreviewsDir  = store.PreviewsDir
	HeadsDir     = store.HeadsDir
	TagsDir      = store.TagsDir
	LogsDir      = store.LogsDir
	HeadFile     = store.HeadFile
	IndexFile    = store.IndexFile
	ConfigFile   = store.ConfigFile
	PacksDir     = store.PacksDir
)

// mapOSError maps an OS-level error to the corresponding storage sentinel.
// os.ErrNotExist maps to store.ErrNotFound and os.ErrPermission maps to
// store.ErrPermission; all other errors are returned unchanged so callers
// can wrap them with additional context. Callers that handle not-exist
// specially (e.g. returning a default value) should do so before calling
// this helper.
func mapOSError(err error) error {
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return store.ErrNotFound
	}
	if os.IsPermission(err) {
		return store.ErrPermission
	}
	return err
}

type FSStorage struct {
	root string
	// zstdEncoderPool recycles zstd.Encoder instances. Each encoder is not
	// safe for concurrent use, but the pool permits N concurrent goroutines
	// to each acquire an encoder without blocking on a single lock.
	zstdEncoderPool sync.Pool
	zstdDecoder     *zstd.Decoder
	compression     bool
	// packMu protects the packIndices cache. save's worker pool may
	// concurrently call GetChunk and trigger lazy pack index loading, so
	// packIndices needs mutex protection.
	packMu      sync.Mutex
	packIndices map[string]*packIndex
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
		filepath.Join(root, LogsDir),
		filepath.Join(root, ChunksDir, PacksDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, fsutil.DefaultDirPerm); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("create zstd decoder: %w", err)
	}

	fs := &FSStorage{
		root:        root,
		zstdDecoder: decoder,
		compression: true,
		packIndices: make(map[string]*packIndex),
	}
	fs.zstdEncoderPool.New = func() any {
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return nil
		}
		return enc
	}
	return fs, nil
}

func (fs *FSStorage) Close() error {
	fs.zstdDecoder.Close()
	return nil
}

// SetCompressionConfig applies the compression settings to the backend.
// enabled toggles zstd compression of chunk payloads; level is the zstd
// command-line level (1-19) and is converted to the library's EncoderLevel
// internally. This satisfies store.ConfigStorer so porcelain can apply
// config uniformly across backends without type-asserting to FSStorage.
func (fs *FSStorage) SetCompressionConfig(enabled bool, level int) error {
	fs.compression = enabled
	if enabled {
		fs.zstdEncoderPool.New = func() any {
			enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
			if err != nil {
				return nil
			}
			return enc
		}
	}
	return nil
}

// isCompressionEnabled returns whether zstd compression is currently enabled.
func (fs *FSStorage) isCompressionEnabled() bool {
	return fs.compression
}

// buildChunkPayload builds the wire-format payload (1-byte header + data)
// for a chunk. If tryCompress is true, the data is zstd-compressed; if the
// compressed form is not smaller than the original, the uncompressed data
// is stored instead (header bit left clear). Returns the payload and the
// header flags byte. Acquires zstdMu only when compression is attempted.
//
// For loose chunks, pass fs.isCompressionEnabled() as tryCompress.
// For pack entries, pass (chunk.Flags == core.ChunkFlagCompressed).
func (fs *FSStorage) buildChunkPayload(data []byte, tryCompress bool) (payload []byte, flags byte) {
	if !tryCompress {
		payload = make([]byte, 0, store.ChunkHeaderSize+len(data))
		payload = append(payload, 0x00)
		payload = append(payload, data...)
		return payload, 0x00
	}
	enc, _ := fs.zstdEncoderPool.Get().(*zstd.Encoder)
	if enc == nil {
		enc, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if enc == nil {
			payload = make([]byte, 0, store.ChunkHeaderSize+len(data))
			payload = append(payload, 0x00)
			payload = append(payload, data...)
			return payload, 0x00
		}
	}
	defer fs.zstdEncoderPool.Put(enc)

	compressed := enc.EncodeAll(data, nil)
	if len(compressed) >= len(data) {
		payload = make([]byte, 0, store.ChunkHeaderSize+len(data))
		payload = append(payload, 0x00)
		payload = append(payload, data...)
		return payload, 0x00
	}
	payload = make([]byte, 0, store.ChunkHeaderSize+len(compressed))
	payload = append(payload, store.ChunkFlagCompressed)
	payload = append(payload, compressed...)
	return payload, store.ChunkFlagCompressed
}

// maxDecompressedChunkSize bounds the decompressed output of a single chunk
// to prevent a zstd decompression bomb (high-compression-ratio payload) from
// exhausting memory. The chunker's MaxChunkSize is the legitimate upper
// bound on a chunk's decompressed size; we use a generous ceiling above that.
const maxDecompressedChunkSize = 64 << 20 // 64 MB

// decompressFromReader reads zstd-compressed data from r and returns the
// decoded bytes. Acquires zstdMu internally so it is safe for concurrent
// use with other zstd operations.
//
// The decompressed output is capped at maxDecompressedChunkSize to prevent
// a decompression bomb from exhausting memory: a small compressed input
// could otherwise expand to an unbounded buffer via io.ReadAll.
func (fs *FSStorage) decompressFromReader(r io.Reader) ([]byte, error) {
	// zstd.Decoder.Reset is not concurrency-safe, but in practice
	// decompression of different chunks happens sequentially within a
	// single goroutine (the save worker pool calls GetChunk one chunk at
	// a time). If concurrent decompression is ever needed, wrap the
	// decoder in a sync.Pool similar to the encoder.
	if err := fs.zstdDecoder.Reset(r); err != nil {
		return nil, err
	}
	limited := &io.LimitedReader{R: fs.zstdDecoder, N: maxDecompressedChunkSize + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		if errors.Is(err, io.EOF) && limited.N == 0 {
			return nil, fmt.Errorf("decompressed chunk exceeds max size %d: %w", maxDecompressedChunkSize, store.ErrCorrupted)
		}
		return nil, err
	}
	if int64(len(data)) > maxDecompressedChunkSize {
		return nil, fmt.Errorf("decompressed chunk exceeds max size %d: %w", maxDecompressedChunkSize, store.ErrCorrupted)
	}
	return data, nil
}

// GetPreview is a noop stub (Phase 1).
func (fs *FSStorage) GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error) {
	return nil, fmt.Errorf("get preview %s: %w", hash.FullString(), store.ErrNotFound)
}

// PutPreview is a noop stub (Phase 1).
func (fs *FSStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
	return nil
}
