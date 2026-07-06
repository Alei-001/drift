package chunker

import (
	"bytes"
	"context"
	"testing"
)

// generateTestData produces size bytes of deterministic pseudo-random data
// using a simple LCG. Deterministic data makes chunk-size assertions
// reproducible across runs while avoiding repetitive patterns that could
// skew CDC cut points.
func generateTestData(size int, seed uint64) []byte {
	data := make([]byte, size)
	state := seed
	for i := range data {
		state = state*6364136223846793005 + 1442695040888963407
		data[i] = byte(state >> 32)
	}
	return data
}

// TestNewFastCDCChunker_Default verifies the default constructor still uses
// the historical 128KB/256KB/512KB sizes, preserving backward compatibility.
func TestNewFastCDCChunker_Default(t *testing.T) {
	c := NewFastCDCChunker()
	if c.minSize != 128*1024 {
		t.Errorf("default minSize = %d, want %d", c.minSize, 128*1024)
	}
	if c.avgSize != 256*1024 {
		t.Errorf("default avgSize = %d, want %d", c.avgSize, 256*1024)
	}
	if c.maxSize != 512*1024 {
		t.Errorf("default maxSize = %d, want %d", c.maxSize, 512*1024)
	}
}

// TestNewFastCDCChunkerWithParams_SmallText verifies that small (4K-16K)
// parameters are honored and produce chunks within the requested range.
func TestNewFastCDCChunkerWithParams_SmallText(t *testing.T) {
	const minSize, avgSize, maxSize = 4096, 8192, 16384
	c := NewFastCDCChunkerWithParams(minSize, avgSize, maxSize)
	if c.minSize != minSize || c.avgSize != avgSize || c.maxSize != maxSize {
		t.Fatalf("params not preserved: got (%d,%d,%d), want (%d,%d,%d)",
			c.minSize, c.avgSize, c.maxSize, minSize, avgSize, maxSize)
	}

	// 200KB of deterministic data (>> maxSize so multiple chunks are produced).
	data := generateTestData(200*1024, 42)
	chunks, err := c.Chunk(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	// FastCDC guarantees every chunk <= maxSize; every chunk except possibly
	// the last is >= minSize (the trailing chunk may be shorter than minSize).
	for i, ch := range chunks {
		if ch.Size > uint32(maxSize) {
			t.Errorf("chunk %d: size %d exceeds maxSize %d", i, ch.Size, maxSize)
		}
		if i < len(chunks)-1 && ch.Size < uint32(minSize) {
			t.Errorf("chunk %d: size %d below minSize %d", i, ch.Size, minSize)
		}
	}

	var total uint32
	for _, ch := range chunks {
		total += ch.Size
	}
	if total != uint32(len(data)) {
		t.Errorf("total chunk size %d != input size %d", total, len(data))
	}
}

// TestNewFastCDCChunkerWithParams_LargeBinary verifies that large (1M-4M)
// parameters are honored and produce chunks within the requested range.
func TestNewFastCDCChunkerWithParams_LargeBinary(t *testing.T) {
	const minSize, avgSize, maxSize = 1024 * 1024, 2 * 1024 * 1024, 4 * 1024 * 1024
	c := NewFastCDCChunkerWithParams(minSize, avgSize, maxSize)
	if c.minSize != minSize || c.avgSize != avgSize || c.maxSize != maxSize {
		t.Fatalf("params not preserved: got (%d,%d,%d), want (%d,%d,%d)",
			c.minSize, c.avgSize, c.maxSize, minSize, avgSize, maxSize)
	}

	// 10MB of deterministic data (>> maxSize so multiple chunks are produced).
	data := generateTestData(10*1024*1024, 7)
	chunks, err := c.Chunk(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}

	for i, ch := range chunks {
		if ch.Size > uint32(maxSize) {
			t.Errorf("chunk %d: size %d exceeds maxSize %d", i, ch.Size, maxSize)
		}
		if i < len(chunks)-1 && ch.Size < uint32(minSize) {
			t.Errorf("chunk %d: size %d below minSize %d", i, ch.Size, minSize)
		}
	}

	var total uint32
	for _, ch := range chunks {
		total += ch.Size
	}
	if total != uint32(len(data)) {
		t.Errorf("total chunk size %d != input size %d", total, len(data))
	}
}

// TestNewFastCDCChunkerWithParams_InvalidParams verifies that illegal inputs
// (zeros, negatives, min > max) are clamped to values the library accepts and
// that the resulting chunker runs without error.
func TestNewFastCDCChunkerWithParams_InvalidParams(t *testing.T) {
	cases := []struct {
		name                      string
		minSize, avgSize, maxSize int
	}{
		{"all zero", 0, 0, 0},
		{"all negative", -1, -1, -1},
		{"min greater than max", 1024 * 1024, 2 * 1024 * 1024, 512 * 1024},
		{"negative min only", -100, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Constructor must not panic.
			c := NewFastCDCChunkerWithParams(tc.minSize, tc.avgSize, tc.maxSize)

			// Resulting params must satisfy the FastCDC library constraints.
			if c.minSize < fastCDCMinSizeFloor {
				t.Errorf("clamped minSize %d < floor %d", c.minSize, fastCDCMinSizeFloor)
			}
			if !isPowerOfTwo(c.avgSize) {
				t.Errorf("clamped avgSize %d is not a power of two", c.avgSize)
			}
			if c.minSize >= c.avgSize {
				t.Errorf("minSize %d >= avgSize %d", c.minSize, c.avgSize)
			}
			if c.avgSize >= c.maxSize {
				t.Errorf("avgSize %d >= maxSize %d", c.avgSize, c.maxSize)
			}

			// Chunk() must work on the clamped params.
			data := generateTestData(256*1024, 1)
			chunks, err := c.Chunk(context.Background(), bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Chunk failed on clamped params: %v", err)
			}
			var total uint32
			for _, ch := range chunks {
				total += ch.Size
			}
			if total != uint32(len(data)) {
				t.Errorf("total chunk size %d != input size %d", total, len(data))
			}
		})
	}
}

// TestNewFastCDCChunkerWithParams_PreservesData verifies that data round-trips
// correctly (reassembled == original) across several parameter configurations.
func TestNewFastCDCChunkerWithParams_PreservesData(t *testing.T) {
	configs := []struct {
		name                      string
		minSize, avgSize, maxSize int
		dataSize                  int
	}{
		{"small", 4096, 8192, 16384, 200 * 1024},
		{"default", 128 * 1024, 256 * 1024, 512 * 1024, 1024 * 1024},
		{"large", 1024 * 1024, 2 * 1024 * 1024, 4 * 1024 * 1024, 10 * 1024 * 1024},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			original := generateTestData(tc.dataSize, 99)
			c := NewFastCDCChunkerWithParams(tc.minSize, tc.avgSize, tc.maxSize)
			chunks, err := c.Chunk(context.Background(), bytes.NewReader(original))
			if err != nil {
				t.Fatalf("Chunk failed: %v", err)
			}

			var reassembled []byte
			for _, ch := range chunks {
				reassembled = append(reassembled, ch.Data...)
			}
			if !bytes.Equal(original, reassembled) {
				t.Errorf("roundtrip failed: reassembled data does not match original")
			}
		})
	}
}
