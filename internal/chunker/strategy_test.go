package chunker

import (
	"testing"

	"github.com/your-org/drift/internal/core"
)

func TestBinaryChunkerFor_SmallFile(t *testing.T) {
	// < 50MB: FastCDC with default params.
	c := BinaryChunkerFor(10*1024*1024, nil)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("10MB: expected *FastCDCChunker, got %T", c)
	}
}

func TestBinaryChunkerFor_LargeFile(t *testing.T) {
	// 50MB <= size < 500MB: FastCDC with scaled params.
	c := BinaryChunkerFor(100*1024*1024, nil)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("100MB: expected *FastCDCChunker, got %T", c)
	}
}

func TestBinaryChunkerFor_HugeFile(t *testing.T) {
	// >= 500MB: FixedChunker.
	c := BinaryChunkerFor(600*1024*1024, nil)
	if _, ok := c.(*FixedChunker); !ok {
		t.Errorf("600MB: expected *FixedChunker, got %T", c)
	}
}

func TestBinaryChunkerFor_Boundaries(t *testing.T) {
	// 50MB - 1 byte: small tier (FastCDC default).
	c := BinaryChunkerFor(50*1024*1024-1, nil)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("50MB-1: expected *FastCDCChunker, got %T", c)
	}
	// Exactly 50MB: large tier (FastCDC scaled).
	c = BinaryChunkerFor(50*1024*1024, nil)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("50MB: expected *FastCDCChunker, got %T", c)
	}
	// 500MB - 1 byte: large tier (FastCDC scaled).
	c = BinaryChunkerFor(500*1024*1024-1, nil)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("500MB-1: expected *FastCDCChunker, got %T", c)
	}
	// Exactly 500MB: huge tier (FixedChunker).
	c = BinaryChunkerFor(500*1024*1024, nil)
	if _, ok := c.(*FixedChunker); !ok {
		t.Errorf("500MB: expected *FixedChunker, got %T", c)
	}
}

func TestBinaryChunkerFor_NilCfg(t *testing.T) {
	// nil cfg should fall back to defaults without panicking.
	c := BinaryChunkerFor(1024, nil)
	if c == nil {
		t.Fatal("expected non-nil chunker for nil cfg")
	}
}

func TestBinaryChunkerFor_CustomCfg(t *testing.T) {
	cfg := &core.CoreConfig{
		ChunkMinSize: 64 * 1024,
		ChunkAvgSize: 128 * 1024,
		ChunkMaxSize: 256 * 1024,
	}
	c := BinaryChunkerFor(10*1024*1024, cfg)
	fc, ok := c.(*FastCDCChunker)
	if !ok {
		t.Fatalf("expected *FastCDCChunker, got %T", c)
	}
	// Custom min size should be honored (clamped to floor 64).
	if fc.minSize != 64*1024 {
		t.Errorf("minSize: got %d, want %d", fc.minSize, 64*1024)
	}
	if fc.maxSize != 256*1024 {
		t.Errorf("maxSize: got %d, want %d", fc.maxSize, 256*1024)
	}
}

func TestBinaryChunkerFor_ZeroChunkSizesInCfg(t *testing.T) {
	// Zero chunk sizes in cfg mean "use engine default".
	cfg := &core.CoreConfig{
		ChunkMinSize: 0,
		ChunkAvgSize: 0,
		ChunkMaxSize: 0,
	}
	c := BinaryChunkerFor(10*1024*1024, cfg)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("expected *FastCDCChunker for zero cfg, got %T", c)
	}
}

func TestDefaultSelector_ChunkerFor(t *testing.T) {
	s := DefaultSelector{}
	c := s.ChunkerFor(10*1024*1024, nil)
	if c == nil {
		t.Fatal("expected non-nil chunker from DefaultSelector")
	}
	// Should match BinaryChunkerFor for the same input.
	expected := BinaryChunkerFor(10*1024*1024, nil)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("expected *FastCDCChunker, got %T", c)
	}
	if _, ok := expected.(*FastCDCChunker); !ok {
		t.Errorf("expected *FastCDCChunker for BinaryChunkerFor, got %T", expected)
	}
}

func TestBinaryChunkerFor_LargeFileScalesParams(t *testing.T) {
	// For large files (50MB-500MB), params are scaled 4x.
	cfg := &core.CoreConfig{
		ChunkMinSize: 64 * 1024,
		ChunkAvgSize: 128 * 1024,
		ChunkMaxSize: 256 * 1024,
	}
	c := BinaryChunkerFor(100*1024*1024, cfg)
	fc, ok := c.(*FastCDCChunker)
	if !ok {
		t.Fatalf("expected *FastCDCChunker, got %T", c)
	}
	if fc.minSize != 64*1024*4 {
		t.Errorf("scaled minSize: got %d, want %d", fc.minSize, 64*1024*4)
	}
	if fc.maxSize != 256*1024*4 {
		t.Errorf("scaled maxSize: got %d, want %d", fc.maxSize, 256*1024*4)
	}
}

func TestBinaryChunkerFor_HugeFileUsesAvgScaled(t *testing.T) {
	// For huge files (>= 500MB), FixedChunker with avgSize*8.
	cfg := &core.CoreConfig{
		ChunkAvgSize: 256 * 1024,
	}
	c := BinaryChunkerFor(600*1024*1024, cfg)
	fc, ok := c.(*FixedChunker)
	if !ok {
		t.Fatalf("expected *FixedChunker, got %T", c)
	}
	// avgSize * 8 = 256KB * 8 = 2MB; FixedChunker clamps to [4096, 64MB].
	if fc.chunkSize != 256*1024*8 {
		t.Errorf("chunkSize: got %d, want %d", fc.chunkSize, 256*1024*8)
	}
}
