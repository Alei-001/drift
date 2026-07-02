package chunker

import (
	"bytes"
	"strings"
	"testing"
)

func TestFastCDCChunker_ProducesChunks(t *testing.T) {
	data := []byte(strings.Repeat("Hello, World! This is a test of content-defined chunking. ", 10000))
	chunker := NewFastCDCChunker()

	chunks, err := chunker.Chunk(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("FastCDC Chunk failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk, got none")
	}

	// Verify total size matches original
	var totalSize uint32
	for _, c := range chunks {
		totalSize += c.Size
		if c.Size == 0 {
			t.Error("chunk has zero size")
		}
		if len(c.Data) != int(c.Size) {
			t.Errorf("chunk Data length %d != Size %d", len(c.Data), c.Size)
		}
	}
	if totalSize != uint32(len(data)) {
		t.Errorf("total chunk size %d != original size %d", totalSize, len(data))
	}
}

func TestFixedChunker_CorrectChunkSize(t *testing.T) {
	chunkSize := 4096
	data := make([]byte, 5000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	chunker := NewFixedChunker(chunkSize)

	chunks, err := chunker.Chunk(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Fixed Chunk failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk, got none")
	}

	// All chunks except possibly the last should have exactly chunkSize
	for i, c := range chunks {
		if i < len(chunks)-1 {
			if c.Size != uint32(chunkSize) {
				t.Errorf("chunk %d: expected size %d, got %d", i, chunkSize, c.Size)
			}
		}
		if len(c.Data) != int(c.Size) {
			t.Errorf("chunk %d: Data length %d != Size %d", i, len(c.Data), c.Size)
		}
	}

	// Verify total size matches original
	var totalSize uint32
	for _, c := range chunks {
		totalSize += c.Size
	}
	if totalSize != uint32(len(data)) {
		t.Errorf("total chunk size %d != original size %d", totalSize, len(data))
	}
}

func TestFixedChunker_MinimumChunkSize(t *testing.T) {
	// FixedChunker(100) should be clamped to a minimum of 4096 bytes.
	// Verify that all chunks produced are at least 4096 bytes (except the last).
	data := make([]byte, 12000)
	for i := range data {
		data[i] = byte(i % 256)
	}
	chunker := NewFixedChunker(100)
	chunks, err := chunker.Chunk(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks for 12000 bytes with min 4096, got %d", len(chunks))
	}
	for i, c := range chunks {
		if i < len(chunks)-1 {
			if c.Size < 4096 {
				t.Errorf("chunk %d: expected size >= 4096, got %d", i, c.Size)
			}
		}
	}
}

func TestRoundTrip_FixedChunker(t *testing.T) {
	original := []byte("The quick brown fox jumps over the lazy dog. 1234567890!@#$%^&*()")
	chunker := NewFixedChunker(16)

	chunks, err := chunker.Chunk(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("Fixed Chunk failed: %v", err)
	}

	var reassembled []byte
	for _, c := range chunks {
		reassembled = append(reassembled, c.Data...)
	}

	if !bytes.Equal(original, reassembled) {
		t.Errorf("roundtrip failed: reassembled data doesn't match original")
	}
}

func TestRoundTrip_FastCDCChunker(t *testing.T) {
	original := []byte(strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", 1000))
	chunker := NewFastCDCChunker()

	chunks, err := chunker.Chunk(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("FastCDC Chunk failed: %v", err)
	}

	var reassembled []byte
	for _, c := range chunks {
		reassembled = append(reassembled, c.Data...)
	}

	if !bytes.Equal(original, reassembled) {
		t.Errorf("roundtrip failed: reassembled data doesn't match original")
	}
}

func TestFastCDCChunker_EmptyData(t *testing.T) {
	chunker := NewFastCDCChunker()
	chunks, err := chunker.Chunk(bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("FastCDC Chunk failed on empty data: %v", err)
	}
	if chunks != nil {
		t.Errorf("expected nil chunks for empty data, got %d chunks", len(chunks))
	}
}

func TestFixedChunker_EmptyData(t *testing.T) {
	chunker := NewFixedChunker(1024)
	chunks, err := chunker.Chunk(bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("Fixed Chunk failed on empty data: %v", err)
	}
	if chunks != nil {
		// Empty data should produce no chunks
		if len(chunks) > 0 {
			t.Errorf("expected no chunks for empty data, got %d chunks", len(chunks))
		}
	}
}

func TestFastCDCChunker_SingleByte(t *testing.T) {
	data := []byte{0x42}
	chunker := NewFastCDCChunker()

	chunks, err := chunker.Chunk(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("FastCDC Chunk failed on single byte: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for single byte, got %d", len(chunks))
	}
	if len(chunks[0].Data) != 1 {
		t.Fatalf("expected chunk Data length 1, got %d", len(chunks[0].Data))
	}
	if chunks[0].Data[0] != 0x42 {
		t.Errorf("expected chunk Data [0x42], got [0x%x]", chunks[0].Data[0])
	}
	if chunks[0].Size != 1 {
		t.Errorf("expected chunk Size 1, got %d", chunks[0].Size)
	}
}

func TestFixedChunker_SingleByte(t *testing.T) {
	data := []byte{0x42}
	chunker := NewFixedChunker(4096)

	chunks, err := chunker.Chunk(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Fixed Chunk failed on single byte: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for single byte, got %d", len(chunks))
	}
	if len(chunks[0].Data) != 1 {
		t.Fatalf("expected chunk Data length 1, got %d", len(chunks[0].Data))
	}
	if chunks[0].Data[0] != 0x42 {
		t.Errorf("expected chunk Data [0x42], got [0x%x]", chunks[0].Data[0])
	}
	if chunks[0].Size != 1 {
		t.Errorf("expected chunk Size 1, got %d", chunks[0].Size)
	}
}
