package filesystem

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/zeebo/blake3"
)

const (
	packIndexMagic      = "DPID"
	packIndexVersion    = 1
	packThreshold       = 512
	packRewriteRatio    = 0.5
	packPrefix          = "pack-"
	packNameFormat      = packPrefix + "%08d"
	packEntrySize       = 45
	packIndexHeaderSize = 4 + 1 + 4 // magic + version + chunk_count
)

// packEntry records where a single chunk lives inside a pack file.
type packEntry struct {
	offset int64
	length uint32
	flags  byte
}

// packIndex is the in-memory representation of a pack's .idx file.
type packIndex struct {
	name    string
	entries map[core.Hash]packEntry
}

func (fs *FSStorage) packsDir() string {
	return filepath.Join(fs.root, ChunksDir, PacksDir)
}

func (fs *FSStorage) packPath(name string) string {
	return filepath.Join(fs.packsDir(), name+".pack")
}

func (fs *FSStorage) packIndexPath(name string) string {
	return filepath.Join(fs.packsDir(), name+".idx")
}

// nextPackName scans the packs directory and returns a new pack name
// with an incremented sequence number.
func (fs *FSStorage) nextPackName() (string, error) {
	entries, err := os.ReadDir(fs.packsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf(packNameFormat, 1), nil
		}
		return "", fmt.Errorf("read packs dir: %w", err)
	}

	maxN := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".pack") {
			continue
		}
		base := strings.TrimSuffix(name, ".pack")
		if !strings.HasPrefix(base, packPrefix) {
			continue
		}
		numStr := base[len(packPrefix):]
		n, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		if n > maxN {
			maxN = n
		}
	}
	return fmt.Sprintf(packNameFormat, maxN+1), nil
}

// writePackIndex writes the packIndex to a .idx file using atomic write.
func (fs *FSStorage) writePackIndex(name string, idx *packIndex) error {
	buf := new(bytes.Buffer)
	buf.WriteString(packIndexMagic)
	buf.WriteByte(packIndexVersion)
	count := uint32(len(idx.entries))
	if err := binary.Write(buf, binary.BigEndian, count); err != nil {
		return fmt.Errorf("encode chunk count: %w", err)
	}

	keys := make([]core.Hash, 0, len(idx.entries))
	for h := range idx.entries {
		keys = append(keys, h)
	}
	sort.Slice(keys, func(i, j int) bool {
		return bytes.Compare(keys[i][:], keys[j][:]) < 0
	})

	for _, hash := range keys {
		entry := idx.entries[hash]
		buf.Write(hash[:])
		if err := binary.Write(buf, binary.BigEndian, entry.offset); err != nil {
			return fmt.Errorf("encode offset: %w", err)
		}
		if err := binary.Write(buf, binary.BigEndian, entry.length); err != nil {
			return fmt.Errorf("encode length: %w", err)
		}
		buf.WriteByte(entry.flags)
	}

	path := fs.packIndexPath(name)
	if err := fsutil.WriteFileAtomic(path, buf.Bytes(), fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("write pack index %s: %w", name, err)
	}
	return nil
}

// loadPackIndex reads a .idx file from disk and reconstructs the packIndex.
func (fs *FSStorage) loadPackIndex(name string) (*packIndex, error) {
	idxPath := fs.packIndexPath(name)
	data, err := os.ReadFile(idxPath)
	if err != nil {
		return nil, fmt.Errorf("read pack index %s: %w", name, err)
	}
	if len(data) < packIndexHeaderSize {
		return nil, fmt.Errorf("pack index %s: file too short: %w", name, storage.ErrCorrupted)
	}

	if string(data[:4]) != packIndexMagic {
		return nil, fmt.Errorf("pack index %s: bad magic: %w", name, storage.ErrCorrupted)
	}
	if data[4] != packIndexVersion {
		return nil, fmt.Errorf("pack index %s: unsupported version %d: %w", name, data[4], storage.ErrCorrupted)
	}

	count := binary.BigEndian.Uint32(data[5:9])
	expectedSize := packIndexHeaderSize + int(count)*packEntrySize
	if len(data) < expectedSize {
		return nil, fmt.Errorf("pack index %s: file too short for %d entries: %w", name, count, storage.ErrCorrupted)
	}

	entries := make(map[core.Hash]packEntry, count)
	pos := packIndexHeaderSize
	for i := uint32(0); i < count; i++ {
		var hash core.Hash
		copy(hash[:], data[pos:pos+32])
		pos += 32
		offset := int64(binary.BigEndian.Uint64(data[pos : pos+8]))
		pos += 8
		length := binary.BigEndian.Uint32(data[pos : pos+4])
		pos += 4
		flags := data[pos]
		pos++
		entries[hash] = packEntry{offset: offset, length: length, flags: flags}
	}

	return &packIndex{name: name, entries: entries}, nil
}

// getPackIndex returns the in-memory packIndex for name, loading it from
// disk if necessary. The result is cached for subsequent calls.
func (fs *FSStorage) getPackIndex(name string) (*packIndex, error) {
	fs.packMu.Lock()
	if idx, ok := fs.packIndices[name]; ok {
		fs.packMu.Unlock()
		return idx, nil
	}
	fs.packMu.Unlock()

	idx, err := fs.loadPackIndex(name)
	if err != nil {
		return nil, err
	}

	fs.packMu.Lock()
	if cached, ok := fs.packIndices[name]; ok {
		fs.packMu.Unlock()
		return cached, nil
	}
	fs.packIndices[name] = idx
	fs.packMu.Unlock()

	return idx, nil
}

// readChunkFromPack reads a single chunk from a pack file given its entry,
// performs BLAKE3 integrity verification against the expected hash, and
// returns the chunk.
func (fs *FSStorage) readChunkFromPack(name string, entry packEntry, hash core.Hash) (*core.Chunk, error) {
	f, err := os.Open(fs.packPath(name))
	if err != nil {
		return nil, fmt.Errorf("open pack %s: %w", name, err)
	}
	defer f.Close()

	if _, err := f.Seek(entry.offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek pack %s: %w", name, err)
	}

	buf := make([]byte, entry.length)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, fmt.Errorf("read pack %s: %w", name, storage.ErrCorrupted)
	}

	header := buf[0]
	payload := buf[1:]
	compressed := header&chunkFlagCompressed != 0

	var data []byte
	if compressed {
		if err := fs.zstdDecoder.Reset(bytes.NewReader(payload)); err != nil {
			return nil, fmt.Errorf("decode chunk from pack %s: %w", name, storage.ErrCorrupted)
		}
		decoded, err := io.ReadAll(fs.zstdDecoder)
		if err != nil {
			return nil, fmt.Errorf("decode chunk from pack %s: %w", name, storage.ErrCorrupted)
		}
		data = decoded
	} else {
		data = payload
	}

	computedHash := core.Hash(blake3.Sum256(data))
	if computedHash != hash {
		return nil, fmt.Errorf("pack chunk %x integrity check failed: expected %s, got %s: %w", hash[:8], hash.FullString(), computedHash.FullString(), storage.ErrCorrupted)
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
	return ch, nil
}

// listPackNames scans the packs directory and returns all pack names
// (without the .pack extension). Returns nil if the directory does not
// exist.
func (fs *FSStorage) listPackNames() ([]string, error) {
	entries, err := os.ReadDir(fs.packsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read packs dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".pack") {
			continue
		}
		base := strings.TrimSuffix(name, ".pack")
		if !strings.HasPrefix(base, packPrefix) {
			continue
		}
		names = append(names, base)
	}
	return names, nil
}
