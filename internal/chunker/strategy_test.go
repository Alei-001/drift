package chunker

import "testing"

func TestBinaryChunkerFor_SmallFile(t *testing.T) {
	// < 50MB: FastCDC with default params.
	c := BinaryChunkerFor(10 * 1024 * 1024)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("10MB: expected *FastCDCChunker, got %T", c)
	}
}

func TestBinaryChunkerFor_LargeFile(t *testing.T) {
	// 50MB <= size < 500MB: FastCDC with scaled params.
	c := BinaryChunkerFor(100 * 1024 * 1024)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("100MB: expected *FastCDCChunker, got %T", c)
	}
}

func TestBinaryChunkerFor_HugeFile(t *testing.T) {
	// >= 1GB: FastCDC with 8x scaled params.
	c := BinaryChunkerFor(1100 * 1024 * 1024)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("1.1GB: expected *FastCDCChunker, got %T", c)
	}
}

func TestBinaryChunkerFor_Boundaries(t *testing.T) {
	// 50MB - 1 byte: small tier (FastCDC default).
	c := BinaryChunkerFor(50*1024*1024 - 1)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("50MB-1: expected *FastCDCChunker, got %T", c)
	}
	// Exactly 50MB: large tier (FastCDC scaled).
	c = BinaryChunkerFor(50 * 1024 * 1024)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("50MB: expected *FastCDCChunker, got %T", c)
	}
	// 500MB - 1 byte: large tier (FastCDC scaled).
	c = BinaryChunkerFor(500*1024*1024 - 1)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("500MB-1: expected *FastCDCChunker, got %T", c)
	}
	// Exactly 500MB: large tier (FastCDC scaled) — under 1GB.
	c = BinaryChunkerFor(500 * 1024 * 1024)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("500MB: expected *FastCDCChunker, got %T", c)
	}
	// Exactly 1GB: huge tier (FastCDC 8x params).
	c = BinaryChunkerFor(1 * 1024 * 1024 * 1024)
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("1GB: expected *FastCDCChunker, got %T", c)
	}
}

func TestDefaultSelector_ChunkerFor(t *testing.T) {
	s := DefaultSelector{}
	c := s.ChunkerFor(10 * 1024 * 1024)
	if c == nil {
		t.Fatal("expected non-nil chunker from DefaultSelector")
	}
	if _, ok := c.(*FastCDCChunker); !ok {
		t.Errorf("expected *FastCDCChunker, got %T", c)
	}
}

// TestBinaryChunkerFor_DefaultParams verifies the binary-engine default chunk
// sizes (128/256/512 KB via fastCDCDefault* aliases of core.DefaultChunk*Size).
// These defaults are the single source of truth for binary/image/video chunking
// and must not regress.
func TestBinaryChunkerFor_DefaultParams(t *testing.T) {
	c := BinaryChunkerFor(10 * 1024 * 1024) // 10MB file, small tier
	fc, ok := c.(*FastCDCChunker)
	if !ok {
		t.Fatalf("expected *FastCDCChunker, got %T", c)
	}
	if fc.minSize != fastCDCDefaultMinSize {
		t.Errorf("minSize: got %d, want %d (128KB)", fc.minSize, fastCDCDefaultMinSize)
	}
	if fc.avgSize != fastCDCDefaultAvgSize {
		t.Errorf("avgSize: got %d, want %d (256KB)", fc.avgSize, fastCDCDefaultAvgSize)
	}
	if fc.maxSize != fastCDCDefaultMaxSize {
		t.Errorf("maxSize: got %d, want %d (512KB)", fc.maxSize, fastCDCDefaultMaxSize)
	}
}

// TestBinaryChunkerFor_LargeFileScalesParams verifies that the large-file tier
// (200MB–1GB) scales the default params 4×.
func TestBinaryChunkerFor_LargeFileScalesParams(t *testing.T) {
	c := BinaryChunkerFor(300 * 1024 * 1024) // 300MB, large tier
	fc, ok := c.(*FastCDCChunker)
	if !ok {
		t.Fatalf("expected *FastCDCChunker, got %T", c)
	}
	if fc.minSize != fastCDCDefaultMinSize*4 {
		t.Errorf("scaled minSize: got %d, want %d", fc.minSize, fastCDCDefaultMinSize*4)
	}
	if fc.maxSize != fastCDCDefaultMaxSize*4 {
		t.Errorf("scaled maxSize: got %d, want %d", fc.maxSize, fastCDCDefaultMaxSize*4)
	}
}

// TestBinaryChunkerFor_HugeFileUsesAvgScaled verifies that the huge-file tier
// (>= 1GB) uses a FastCDCChunker with 8x scaled params.
func TestBinaryChunkerFor_HugeFileUsesAvgScaled(t *testing.T) {
	c := BinaryChunkerFor(1100 * 1024 * 1024) // 1.1GB, huge tier
	fc, ok := c.(*FastCDCChunker)
	if !ok {
		t.Fatalf("expected *FastCDCChunker, got %T", c)
	}
	wantMin := fastCDCDefaultMinSize * 8
	if fc.minSize != wantMin {
		t.Errorf("scaled minSize: got %d, want %d", fc.minSize, wantMin)
	}
	wantMax := fastCDCDefaultMaxSize * 8
	if fc.maxSize != wantMax {
		t.Errorf("scaled maxSize: got %d, want %d", fc.maxSize, wantMax)
	}
}
