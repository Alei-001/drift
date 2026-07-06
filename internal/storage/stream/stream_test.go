package stream

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage/backends/memory"
	"github.com/zeebo/blake3"
)

func hashData(data []byte) core.Hash {
	return core.Hash(blake3.Sum256(data))
}

func TestChunkReaderReadsSingleChunk(t *testing.T) {
	ctx := context.Background()
	store := memory.NewMemoryStorage()

	data := []byte("hello world")
	h := hashData(data)
	store.PutChunk(ctx, &core.Chunk{Hash: h, Size: uint32(len(data)), Data: data})

	r := NewChunkReader(ctx, store, []core.Hash{h})
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello world" {
		t.Errorf("expected %q, got %q", "hello world", string(out))
	}
}

func TestChunkReaderReadsMultipleChunks(t *testing.T) {
	ctx := context.Background()
	store := memory.NewMemoryStorage()

	data1 := []byte("abc")
	data2 := []byte("def")
	h1 := hashData(data1)
	h2 := hashData(data2)
	store.PutChunk(ctx, &core.Chunk{Hash: h1, Size: uint32(len(data1)), Data: data1})
	store.PutChunk(ctx, &core.Chunk{Hash: h2, Size: uint32(len(data2)), Data: data2})

	r := NewChunkReader(ctx, store, []core.Hash{h1, h2})
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "abcdef" {
		t.Errorf("expected %q, got %q", "abcdef", string(out))
	}
}

func TestChunkReaderEmptyHashesReturnsEOF(t *testing.T) {
	ctx := context.Background()
	store := memory.NewMemoryStorage()

	r := NewChunkReader(ctx, store, nil)
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty output, got %q", string(out))
	}
}

func TestPeekHeaderExact(t *testing.T) {
	r := bytes.NewReader([]byte("abcdefgh"))
	header, rest, err := PeekHeader(r, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(header) != "abcd" {
		t.Errorf("expected %q, got %q", "abcd", string(header))
	}
	restData, _ := io.ReadAll(rest)
	if string(restData) != "abcdefgh" {
		t.Errorf("expected %q from rest, got %q", "abcdefgh", string(restData))
	}
}

func TestPeekHeaderShortData(t *testing.T) {
	r := bytes.NewReader([]byte("ab"))
	header, rest, err := PeekHeader(r, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(header) != "ab" {
		t.Errorf("expected %q, got %q", "ab", string(header))
	}
	restData, _ := io.ReadAll(rest)
	if string(restData) != "ab" {
		t.Errorf("expected %q from rest, got %q", "ab", string(restData))
	}
}

func TestHashFileContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	data := []byte("hello hash")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h, err := HashFileContent(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := hashData(data)
	if h != expected {
		t.Errorf("hash mismatch: got %s, expected %s", h.String(), expected.String())
	}
}

func TestHashChunkData(t *testing.T) {
	ctx := context.Background()
	store := memory.NewMemoryStorage()

	data1 := []byte("chunk")
	data2 := []byte("data")
	h1 := hashData(data1)
	h2 := hashData(data2)
	store.PutChunk(ctx, &core.Chunk{Hash: h1, Size: uint32(len(data1)), Data: data1})
	store.PutChunk(ctx, &core.Chunk{Hash: h2, Size: uint32(len(data2)), Data: data2})

	h, err := HashChunkData(ctx, store, []core.Hash{h1, h2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := hashData([]byte("chunkdata"))
	if h != expected {
		t.Errorf("hash mismatch: got %s, expected %s", h.String(), expected.String())
	}
}
