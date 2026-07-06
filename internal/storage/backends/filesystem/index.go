package filesystem

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
	"google.golang.org/protobuf/proto"
)

// GetIndex reads the staging index from disk.
func (fs *FSStorage) GetIndex(ctx context.Context) (*core.Index, error) {
	path := filepath.Join(fs.root, IndexFile)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &core.Index{}, nil
		}
		return nil, err
	}
	defer f.Close()
	// Stream the index file through os.Open + io.ReadAll rather than
	// os.ReadFile, and release the raw buffer promptly after unmarshaling.
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	p := &core.IndexProto{}
	if err := proto.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("unmarshal index: %w", storage.ErrCorrupted)
	}
	data = nil
	return indexFromProto(p), nil
}

// SetIndex writes the staging index to disk.
func (fs *FSStorage) SetIndex(ctx context.Context, index *core.Index) error {
	p := indexToProto(index)
	data, err := proto.Marshal(p)
	if err != nil {
		return err
	}
	path := filepath.Join(fs.root, IndexFile)
	return fsutil.WriteFileAtomic(path, data, 0644)
}

// --- protobuf conversion helpers ---

func indexToProto(idx *core.Index) *core.IndexProto {
	if idx == nil {
		return nil
	}
	p := &core.IndexProto{
		UpdatedAt: idx.UpdatedAt,
	}
	for _, e := range idx.Entries {
		ep := &core.IndexEntryProto{
			Path:    e.Path,
			Size:    e.Size,
			ModTime: e.ModTime,
		}
		ep.Hash = make([]byte, len(e.Hash))
		copy(ep.Hash, e.Hash[:])
		for _, ch := range e.Chunks {
			ep.ChunkHashes = append(ep.ChunkHashes, copyBytes(ch[:]))
		}
		p.Entries = append(p.Entries, ep)
	}
	return p
}

func indexFromProto(p *core.IndexProto) *core.Index {
	if p == nil {
		return nil
	}
	idx := &core.Index{
		UpdatedAt: p.UpdatedAt,
	}
	for _, ep := range p.Entries {
		e := core.IndexEntry{
			Path:    ep.Path,
			Size:    ep.Size,
			ModTime: ep.ModTime,
		}
		copy(e.Hash[:], ep.Hash)
		for _, ch := range ep.ChunkHashes {
			e.Chunks = append(e.Chunks, bytesToHash(ch))
		}
		idx.Entries = append(idx.Entries, e)
	}
	return idx
}
