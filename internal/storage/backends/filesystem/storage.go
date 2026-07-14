package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/cache"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/klauspost/compress/zstd"
)

// mapOSError maps an OS-level error to the corresponding storage sentinel.
// os.ErrNotExist maps to storage.ErrNotFound and os.ErrPermission maps to
// storage.ErrPermission; all other errors are returned unchanged so callers
// can wrap them with additional context. Callers that handle not-exist
// specially (e.g. returning a default value) should do so before calling
// this helper.
func mapOSError(err error) error {
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return storage.ErrNotFound
	}
	if os.IsPermission(err) {
		return storage.ErrPermission
	}
	return err
}

type FSStorage struct {
	root       string
	chunkCache *cache.Cache[core.Hash, *core.Chunk]
	// zstdMu guards the zstd encoder/decoder and the compression flag.
	// The storage layer is concurrency-safe on its own; the porcelain
	// workspace lock is for higher-level atomicity, not for protecting
	// storage-internal shared state. save's worker pool may concurrently
	// call PutChunk/GetChunk, so the zstd codec (which is NOT concurrency-
	// safe per klauspost/compress/zstd) must be serialized.
	zstdMu      sync.Mutex
	zstdDecoder *zstd.Decoder
	zstdEncoder *zstd.Encoder
	compression bool
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

	chunkCache, err := cache.NewCache[core.Hash, *core.Chunk](256)
	if err != nil {
		return nil, fmt.Errorf("create chunk cache: %w", err)
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
		root:        root,
		chunkCache:  chunkCache,
		zstdDecoder: decoder,
		zstdEncoder: encoder,
		compression: true,
		packIndices: make(map[string]*packIndex),
	}, nil
}

func (fs *FSStorage) Close() error {
	fs.zstdMu.Lock()
	defer fs.zstdMu.Unlock()
	fs.zstdDecoder.Close()
	if err := fs.zstdEncoder.Close(); err != nil {
		return fmt.Errorf("close zstd encoder: %w", err)
	}
	return nil
}

// SetCompressionConfig applies the compression settings to the backend.
// enabled toggles zstd compression of chunk payloads; level is the zstd
// command-line level (1-19) and is converted to the library's EncoderLevel
// internally. This satisfies storage.ConfigStorer so porcelain can apply
// config uniformly across backends without type-asserting to FSStorage.
func (fs *FSStorage) SetCompressionConfig(enabled bool, level int) error {
	fs.zstdMu.Lock()
	defer fs.zstdMu.Unlock()
	fs.compression = enabled
	if enabled {
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
		if err != nil {
			return fmt.Errorf("create zstd encoder: %w", err)
		}
		fs.zstdEncoder.Close()
		fs.zstdEncoder = enc
	}
	return nil
}

// isCompressionEnabled returns whether zstd compression is currently
// enabled. Safe for concurrent use.
func (fs *FSStorage) isCompressionEnabled() bool {
	fs.zstdMu.Lock()
	defer fs.zstdMu.Unlock()
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
		payload = make([]byte, 0, storage.ChunkHeaderSize+len(data))
		payload = append(payload, 0x00)
		payload = append(payload, data...)
		return payload, 0x00
	}
	fs.zstdMu.Lock()
	defer fs.zstdMu.Unlock()
	compressed := fs.zstdEncoder.EncodeAll(data, nil)
	// If compression makes the data larger, store uncompressed.
	if len(compressed) >= len(data) {
		payload = make([]byte, 0, storage.ChunkHeaderSize+len(data))
		payload = append(payload, 0x00)
		payload = append(payload, data...)
		return payload, 0x00
	}
	payload = make([]byte, 0, storage.ChunkHeaderSize+len(compressed))
	payload = append(payload, storage.ChunkFlagCompressed)
	payload = append(payload, compressed...)
	return payload, storage.ChunkFlagCompressed
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
	fs.zstdMu.Lock()
	defer fs.zstdMu.Unlock()
	if err := fs.zstdDecoder.Reset(r); err != nil {
		return nil, err
	}
	// LimitReader on the decoder's output caps the decompressed size.
	// If the decompressed data exceeds the limit, ReadAll returns an
	// error from the LimitedReader, which we map to ErrCorrupted.
	limited := &io.LimitedReader{R: fs.zstdDecoder, N: maxDecompressedChunkSize + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		if err == io.EOF && limited.N == 0 {
			return nil, fmt.Errorf("decompressed chunk exceeds max size %d: %w", maxDecompressedChunkSize, storage.ErrCorrupted)
		}
		return nil, err
	}
	if int64(len(data)) > maxDecompressedChunkSize {
		return nil, fmt.Errorf("decompressed chunk exceeds max size %d: %w", maxDecompressedChunkSize, storage.ErrCorrupted)
	}
	return data, nil
}
