package filesystem

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
)

// CompactChunks implements storage.ChunkCompactor for the filesystem backend.
func (fs *FSStorage) CompactChunks(ctx context.Context, reachable map[core.Hash]bool, dryRun bool) (storage.CompactReport, error) {
	var report storage.CompactReport

	chunksDir := fs.chunksDir()
	aliveLoose := make(map[core.Hash]struct{})

	err := filepath.WalkDir(chunksDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk chunks: %w", err)
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(chunksDir, path)
			rel = filepath.ToSlash(rel)
			if rel == PacksDir {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(chunksDir, path)
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
		if reachable[h] {
			aliveLoose[h] = struct{}{}
		} else {
			if ch, gerr := fs.GetChunk(ctx, h); gerr == nil {
				report.FreedBytes += int64(ch.Size)
			}
			report.LooseDeleted++
			if !dryRun {
				if err := fs.DeleteChunk(ctx, h); err != nil {
					return fmt.Errorf("delete dead chunk %s: %w", h.FullString(), err)
				}
			}
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return report, fmt.Errorf("walk loose chunks: %w", err)
	}

	aliveLooseList := make([]core.Hash, 0, len(aliveLoose))
	for h := range aliveLoose {
		aliveLooseList = append(aliveLooseList, h)
	}
	if len(aliveLooseList) > packThreshold {
		report.LoosePacked = len(aliveLooseList)
		report.PacksCreated++
		if !dryRun {
			if err := fs.createPack(ctx, aliveLooseList); err != nil {
				return report, fmt.Errorf("create pack: %w", err)
			}
			for _, h := range aliveLooseList {
				if err := fs.DeleteChunk(ctx, h); err != nil {
					return report, fmt.Errorf("delete packed chunk %s: %w", h.FullString(), err)
				}
			}
		}
	}

	packNames, err := fs.listPackNames()
	if err != nil {
		return report, fmt.Errorf("list packs: %w", err)
	}
	for _, name := range packNames {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		idx, err := fs.getPackIndex(name)
		if err != nil {
			continue
		}

		total := len(idx.entries)
		dead := 0
		for h := range idx.entries {
			if !reachable[h] {
				dead++
			}
		}
		if total == 0 {
			continue
		}
		ratio := float64(dead) / float64(total)
		if ratio < packRewriteRatio {
			continue
		}

		deadBytes := int64(0)
		for h, entry := range idx.entries {
			if !reachable[h] {
				deadBytes += int64(entry.length)
			}
		}
		report.FreedBytes += deadBytes
		report.PackDeadRemoved += dead
		report.PacksRewritten++

		if !dryRun {
			if err := fs.rewritePack(ctx, name, reachable); err != nil {
				return report, fmt.Errorf("rewrite pack %s: %w", name, err)
			}
		}
	}

	return report, nil
}

// createPack bundles the specified chunks (loaded from loose storage) into
// a new pack file and its index. The loose files are NOT deleted; callers
// must handle deletion.
func (fs *FSStorage) createPack(ctx context.Context, hashes []core.Hash) error {
	if len(hashes) == 0 {
		return nil
	}

	name, err := fs.nextPackName()
	if err != nil {
		return fmt.Errorf("generate pack name: %w", err)
	}

	var packData bytes.Buffer
	entries := make(map[core.Hash]packEntry, len(hashes))

	for _, hash := range hashes {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := fs.GetChunk(ctx, hash)
		if err != nil {
			return fmt.Errorf("read chunk %x for pack: %w", hash[:8], err)
		}

		offset := int64(packData.Len())
		flags := byte(0x00)
		if chunk.Flags == core.ChunkFlagCompressed {
			flags = chunkFlagCompressed
		}

		var payload []byte
		if flags == chunkFlagCompressed {
			compressed := fs.zstdEncoder.EncodeAll(chunk.Data, nil)
			payload = make([]byte, 0, 1+len(compressed))
			payload = append(payload, chunkFlagCompressed)
			payload = append(payload, compressed...)
		} else {
			payload = make([]byte, 0, 1+len(chunk.Data))
			payload = append(payload, 0x00)
			payload = append(payload, chunk.Data...)
		}
		packData.Write(payload)

		length := uint32(len(payload))
		entries[hash] = packEntry{offset: offset, length: length, flags: flags}
	}

	packPath := fs.packPath(name)
	if err := fsutil.WriteFileAtomic(packPath, packData.Bytes(), fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("write pack %s: %w", name, err)
	}

	idx := &packIndex{name: name, entries: entries}
	if err := fs.writePackIndex(name, idx); err != nil {
		return fmt.Errorf("write pack index %s: %w", name, err)
	}

	fs.packMu.Lock()
	fs.packIndices[name] = idx
	fs.packMu.Unlock()

	return nil
}

// rewritePack creates a new pack with only the reachable chunks from the
// old pack, then replaces the old files. Removing old files is best-effort
// (Windows may block the unlink while a reader holds the fd); orphaned
// files are harmless and cleaned up by the next GC pass.
func (fs *FSStorage) rewritePack(ctx context.Context, name string, reachable map[core.Hash]bool) error {
	idx, err := fs.getPackIndex(name)
	if err != nil {
		return fmt.Errorf("load pack index %s: %w", name, err)
	}

	var alive []core.Hash
	for hash := range idx.entries {
		if reachable[hash] {
			alive = append(alive, hash)
		}
	}

	if len(alive) == 0 {
		_ = os.Remove(fs.packPath(name))
		_ = os.Remove(fs.packIndexPath(name))
		fs.packMu.Lock()
		delete(fs.packIndices, name)
		fs.packMu.Unlock()
		return nil
	}

	newName, err := fs.nextPackName()
	if err != nil {
		return fmt.Errorf("generate pack name: %w", err)
	}

	var packData bytes.Buffer
	entries := make(map[core.Hash]packEntry, len(alive))

	for _, hash := range alive {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := idx.entries[hash]
		chunk, err := fs.readChunkFromPack(name, entry, hash)
		if err != nil {
			return fmt.Errorf("read chunk from pack %s: %w", name, err)
		}

		offset := int64(packData.Len())
		flags := byte(0x00)
		if chunk.Flags == core.ChunkFlagCompressed {
			flags = chunkFlagCompressed
		}

		var payload []byte
		if flags == chunkFlagCompressed {
			compressed := fs.zstdEncoder.EncodeAll(chunk.Data, nil)
			payload = make([]byte, 0, 1+len(compressed))
			payload = append(payload, chunkFlagCompressed)
			payload = append(payload, compressed...)
		} else {
			payload = make([]byte, 0, 1+len(chunk.Data))
			payload = append(payload, 0x00)
			payload = append(payload, chunk.Data...)
		}
		packData.Write(payload)

		length := uint32(len(payload))
		entries[hash] = packEntry{offset: offset, length: length, flags: flags}
	}

	newPackPath := fs.packPath(newName)
	if err := fsutil.WriteFileAtomic(newPackPath, packData.Bytes(), fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("write pack %s: %w", newName, err)
	}

	newIdx := &packIndex{name: newName, entries: entries}
	if err := fs.writePackIndex(newName, newIdx); err != nil {
		return fmt.Errorf("write pack index %s: %w", newName, err)
	}

	_ = os.Remove(fs.packPath(name))
	_ = os.Remove(fs.packIndexPath(name))

	fs.packMu.Lock()
	fs.packIndices[newName] = newIdx
	delete(fs.packIndices, name)
	fs.packMu.Unlock()

	return nil
}
