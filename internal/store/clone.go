package store

import "github.com/Alei-001/drift/internal/core"

func CloneChunk(c *core.Chunk) *core.Chunk {
	if c == nil {
		return nil
	}
	clone := &core.Chunk{
		Hash:  c.Hash,
		Size:  c.Size,
		Flags: c.Flags,
	}
	if c.Data != nil {
		clone.Data = make([]byte, len(c.Data))
		copy(clone.Data, c.Data)
	}
	return clone
}

func CloneFileEntry(f core.FileEntry) core.FileEntry {
	clone := f
	if f.Chunks != nil {
		clone.Chunks = make([]core.Hash, len(f.Chunks))
		copy(clone.Chunks, f.Chunks)
	}
	if f.Metadata != nil {
		m := *f.Metadata
		if f.Metadata.Extra != nil {
			m.Extra = make(map[string]string, len(f.Metadata.Extra))
			for k, v := range f.Metadata.Extra {
				m.Extra[k] = v
			}
		}
		clone.Metadata = &m
	}
	return clone
}

func CloneSnapshot(s *core.Snapshot) *core.Snapshot {
	if s == nil {
		return nil
	}
	clone := &core.Snapshot{
		ID:        s.ID,
		Message:   s.Message,
		Author:    s.Author,
		Timestamp: s.Timestamp,
		TotalSize: s.TotalSize,
	}
	if s.PrevID != nil {
		prev := *s.PrevID
		clone.PrevID = &prev
	}
	if s.Files != nil {
		clone.Files = make([]core.FileEntry, len(s.Files))
		for i, f := range s.Files {
			clone.Files[i] = CloneFileEntry(f)
		}
	}
	if s.Tags != nil {
		clone.Tags = make([]string, len(s.Tags))
		copy(clone.Tags, s.Tags)
	}
	return clone
}
