package filesystem

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/zeebo/blake3"
)

func (fs *FSStorage) chunksDir() string {
	return filepath.Join(fs.root, ChunksDir)
}

func (fs *FSStorage) chunkPath(hash core.Hash) string {
	hex := hash.FullString()
	return filepath.Join(fs.chunksDir(), hex[:2], hex[2:])
}

func (fs *FSStorage) HasChunk(ctx context.Context, hash core.Hash) (bool, error) {
	path := fs.chunkPath(hash)
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat chunk %x: %w", hash[:8], mapOSError(err))
	}

	packNames, err := fs.listPackNames()
	if err != nil {
		return false, fmt.Errorf("list packs: %w", err)
	}
	for _, name := range packNames {
		idx, err := fs.getPackIndex(name)
		if err != nil {
			slog.Warn("failed to read pack index, skipping", "pack", name, "error", err)
			continue
		}
		if _, ok := idx.entries[hash]; ok {
			return true, nil
		}
	}
	return false, nil
}

func (fs *FSStorage) GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error) {
	path := fs.chunkPath(hash)
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()

		header := make([]byte, store.ChunkHeaderSize)
		if _, err := io.ReadFull(f, header); err != nil {
			return nil, fmt.Errorf("read chunk header %x: %w", hash[:8], store.ErrCorrupted)
		}

		compressed := header[0]&store.ChunkFlagCompressed != 0

		var data []byte
		if compressed {
			decoded, err := fs.decompressFromReader(f)
			if err != nil {
				return nil, fmt.Errorf("decode chunk %x: %w", hash[:8], store.ErrCorrupted)
			}
			data = decoded
		} else {
			rawData, err := io.ReadAll(f)
			if err != nil {
				return nil, fmt.Errorf("read chunk data %x: %w", hash[:8], err)
			}
			data = rawData
		}

		computedHash := core.Hash(blake3.Sum256(data))
		if computedHash != hash {
			return nil, fmt.Errorf("chunk %x integrity check failed: expected %s, got %s: %w", hash[:8], hash.FullString(), computedHash.FullString(), store.ErrCorrupted)
		}

		flags := core.ChunkFlagNone
		if compressed {
			flags = core.ChunkFlagCompressed
		}

		ch := &core.Chunk{
			Hash:  hash,
			Size:  uint32(len(data)),
			Data:  data,
			Flags: flags,
		}
		return store.CloneChunk(ch), nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("open chunk %x: %w", hash[:8], mapOSError(err))
	}

	packNames, err := fs.listPackNames()
	if err != nil {
		return nil, fmt.Errorf("list packs: %w", err)
	}
	for _, name := range packNames {
		idx, err := fs.getPackIndex(name)
		if err != nil {
			slog.Warn("failed to read pack index, skipping", "pack", name, "error", err)
			continue
		}
		if entry, ok := idx.entries[hash]; ok {
			ch, err := fs.readChunkFromPack(name, entry, hash)
			if err != nil {
				return nil, err
			}
			return store.CloneChunk(ch), nil
		}
	}

	return nil, fmt.Errorf("get chunk %x: %w", hash[:8], store.ErrNotFound)
}

// PutChunk stores a chunk to disk. The chunk data is verified against its
// declared hash before persisting, so a caller-supplied mismatch can never
// reach disk and cause later GetChunk integrity failures. If the chunk
// already exists (loose or in a pack), PutChunk is a no-op.
//
// Writes are not verified by re-reading; on-disk corruption is detected on
// the subsequent GetChunk via the BLAKE3 integrity check.
func (fs *FSStorage) PutChunk(ctx context.Context, chunk *core.Chunk) error {
	// Verify the chunk data matches its declared hash before persisting,
	// so a caller-supplied mismatch can never reach disk and cause later
	// GetChunk integrity failures.
	computed := core.Hash(blake3.Sum256(chunk.Data))
	if computed != chunk.Hash {
		return fmt.Errorf("put chunk %x: hash mismatch, expected %s, got %s: %w", chunk.Hash[:8], chunk.Hash.FullString(), computed.FullString(), store.ErrCorrupted)
	}

	has, err := fs.HasChunk(ctx, chunk.Hash)
	if err != nil {
		return fmt.Errorf("check chunk existence %x: %w", chunk.Hash[:8], err)
	}
	if has {
		return nil
	}

	path := fs.chunkPath(chunk.Hash)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, fsutil.DefaultDirPerm); err != nil {
		return fmt.Errorf("mkdir chunks: %w", err)
	}

	payload, flags := fs.buildChunkPayload(chunk.Data, fs.isCompressionEnabled())

	if err := fsutil.WriteFileAtomic(path, payload, fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("write chunk: %w", err)
	}

	stored := &core.Chunk{
		Hash:  chunk.Hash,
		Size:  uint32(len(chunk.Data)),
		Data:  make([]byte, len(chunk.Data)),
		Flags: core.ChunkFlagNone,
	}
	copy(stored.Data, chunk.Data)
	if flags&store.ChunkFlagCompressed != 0 {
		stored.Flags = core.ChunkFlagCompressed
	}
	return nil
}

// DeleteChunk removes a loose chunk from disk and evicts it from the
// in-memory cache. It is idempotent: a missing file is not an error.
//
// DeleteChunk only removes loose chunks; packed chunks are not affected
// and must be reclaimed via CompactChunks.
func (fs *FSStorage) DeleteChunk(ctx context.Context, hash core.Hash) error {
	path := fs.chunkPath(hash)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete chunk %x: %w", hash[:8], mapOSError(err))
	}
	return nil
}

func (fs *FSStorage) ListChunks(ctx context.Context) ([]core.Hash, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	chunksDir := fs.chunksDir()
	seen := make(map[core.Hash]bool)
	var hashes []core.Hash
	err := filepath.WalkDir(chunksDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk chunks: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(chunksDir, path)
			if rel == PacksDir {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(chunksDir, path)
		if err != nil {
			return fmt.Errorf("rel path %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		parts := strings.Split(rel, "/")
		if len(parts) != 2 {
			slog.Warn("skipping non-chunk file in chunks directory", "path", path)
			return nil
		}
		b, err := hex.DecodeString(parts[0] + parts[1])
		if err != nil {
			slog.Warn("skipping chunk file with invalid hex name", "path", path, "error", err)
			return nil
		}
		var h core.Hash
		copy(h[:], b)
		hashes = append(hashes, h)
		seen[h] = true
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		if err != nil {
			return nil, fmt.Errorf("list chunks: %w", err)
		}
	}

	packNames, err := fs.listPackNames()
	if err != nil {
		return nil, fmt.Errorf("list packs: %w", err)
	}
	for _, name := range packNames {
		idx, err := fs.getPackIndex(name)
		if err != nil {
			slog.Warn("failed to read pack index, skipping", "pack", name, "error", err)
			continue
		}
		for h := range idx.entries {
			if !seen[h] {
				seen[h] = true
				hashes = append(hashes, h)
			}
		}
	}

	return hashes, nil
}
