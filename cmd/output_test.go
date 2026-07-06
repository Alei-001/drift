package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/your-org/drift/internal/core"
)

type mockChunkStore struct {
	chunks map[core.Hash][]byte
}

func (m *mockChunkStore) GetChunk(_ context.Context, h core.Hash) (*core.Chunk, error) {
	data, ok := m.chunks[h]
	if !ok {
		return nil, fmt.Errorf("chunk not found: %x", h[:8])
	}
	return &core.Chunk{Data: data}, nil
}

// TestCountLinesFromChunks verifies that newlines are counted correctly
// across multiple chunks without concatenating them. This is a regression
// test for OOM: the old code appended all chunk data into a single []byte
// before counting, which would OOM on large files (e.g. 200 MB text).
func TestCountLinesFromChunks(t *testing.T) {
	hash1 := core.Hash{0x01}
	hash2 := core.Hash{0x02}
	hash3 := core.Hash{0x03}

	store := &mockChunkStore{
		chunks: map[core.Hash][]byte{
			hash1: []byte("line1\nline2\n"),       // 2 newlines
			hash2: []byte("line3\nline4"),          // 1 newline
			hash3: []byte("line5\nline6\nline7\n"), // 3 newlines
		},
	}

	entry := core.FileEntry{
		Chunks: []core.Hash{hash1, hash2, hash3},
	}

	count := countLinesFromChunks(context.Background(), store, entry)
	if count != 6 {
		t.Errorf("expected 6 newlines, got %d", count)
	}
}

// TestCountLinesFromChunks_MissingChunk verifies that a missing chunk
// causes the function to return 0 rather than panicking.
func TestCountLinesFromChunks_MissingChunk(t *testing.T) {
	store := &mockChunkStore{
		chunks: map[core.Hash][]byte{},
	}
	entry := core.FileEntry{
		Chunks: []core.Hash{{0x01}},
	}
	count := countLinesFromChunks(context.Background(), store, entry)
	if count != 0 {
		t.Errorf("expected 0 for missing chunk, got %d", count)
	}
}
