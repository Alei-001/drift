package filesystem

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/zeebo/blake3"
)

const (
	chunkHeaderSize          = 1
	chunkFlagCompressed byte = 0x01
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
		return false, fmt.Errorf("stat chunk %x: %w", hash[:8], err)
	}

	packNames, err := fs.listPackNames()
	if err != nil {
		return false, fmt.Errorf("list packs: %w", err)
	}
	for _, name := range packNames {
		idx, err := fs.getPackIndex(name)
		if err != nil {
			continue
		}
		if _, ok := idx.entries[hash]; ok {
			return true, nil
		}
	}
	return false, nil
}

func (fs *FSStorage) GetChunk(ctx context.Context, hash core.Hash) (*core.Chunk, error) {
	if ch, ok := fs.chunkCache.Get(hash); ok {
		return storage.CloneChunk(ch), nil
	}

	path := fs.chunkPath(hash)
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()

		header := make([]byte, chunkHeaderSize)
		if _, err := io.ReadFull(f, header); err != nil {
			return nil, fmt.Errorf("read chunk header %x: %w", hash[:8], storage.ErrCorrupted)
		}

		compressed := header[0]&chunkFlagCompressed != 0

		var data []byte
		if compressed {
			if err := fs.zstdDecoder.Reset(f); err != nil {
				return nil, fmt.Errorf("decode chunk %x: %w", hash[:8], storage.ErrCorrupted)
			}
			decoded, err := io.ReadAll(fs.zstdDecoder)
			if err != nil {
				return nil, fmt.Errorf("decode chunk %x: %w", hash[:8], storage.ErrCorrupted)
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
			return nil, fmt.Errorf("chunk %x integrity check failed: expected %s, got %s: %w", hash[:8], hash.FullString(), computedHash.FullString(), storage.ErrCorrupted)
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
		fs.chunkCache.Add(hash, ch)
		return storage.CloneChunk(ch), nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("open chunk %x: %w", hash[:8], err)
	}

	packNames, err := fs.listPackNames()
	if err != nil {
		return nil, fmt.Errorf("list packs: %w", err)
	}
	for _, name := range packNames {
		idx, err := fs.getPackIndex(name)
		if err != nil {
			continue
		}
		if entry, ok := idx.entries[hash]; ok {
			ch, err := fs.readChunkFromPack(name, entry, hash)
			if err != nil {
				return nil, err
			}
			fs.chunkCache.Add(hash, ch)
			return storage.CloneChunk(ch), nil
		}
	}

	return nil, fmt.Errorf("get chunk %x: %w", hash[:8], storage.ErrNotFound)
}

func (fs *FSStorage) PutChunk(ctx context.Context, chunk *core.Chunk) error {
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

	// fs.compression is read without a lock: the porcelain workspace
	// lock guarantees single-threaded access, and SetCompressionConfig is only
	// called during initialization (before any data-access method).
	useCompression := fs.compression

	var payload []byte
	var flags byte
	if useCompression {
		compressed := fs.zstdEncoder.EncodeAll(chunk.Data, nil)
		payload = make([]byte, 0, chunkHeaderSize+len(compressed))
		payload = append(payload, chunkFlagCompressed)
		payload = append(payload, compressed...)
		flags = chunkFlagCompressed
	} else {
		payload = make([]byte, 0, chunkHeaderSize+len(chunk.Data))
		payload = append(payload, 0x00)
		payload = append(payload, chunk.Data...)
		flags = 0x00
	}

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
	if flags&chunkFlagCompressed != 0 {
		stored.Flags = core.ChunkFlagCompressed
	}
	fs.chunkCache.Add(chunk.Hash, stored)
	return nil
}

func (fs *FSStorage) DeleteChunk(ctx context.Context, hash core.Hash) error {
	path := fs.chunkPath(hash)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete chunk %x: %w", hash[:8], err)
	}
	fs.chunkCache.Remove(hash)
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
			return nil
		}
		b, err := hex.DecodeString(parts[0] + parts[1])
		if err != nil {
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
