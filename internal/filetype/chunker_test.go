package filetype

import (
	"testing"

	"github.com/Alei-001/drift/internal/chunker"
	"github.com/Alei-001/drift/internal/filetype/binary"
	"github.com/Alei-001/drift/internal/filetype/image"
	"github.com/Alei-001/drift/internal/filetype/text"
	"github.com/Alei-001/drift/internal/filetype/video"
)

func TestDefaultSelectorSmallFile(t *testing.T) {
	sel := chunker.DefaultSelector{}
	c := sel.ChunkerFor(1024)
	if c == nil {
		t.Fatal("expected non-nil chunker for small file")
	}
	if _, ok := c.(*chunker.FastCDCChunker); !ok {
		t.Errorf("expected FastCDCChunker, got %T", c)
	}
}

func TestDefaultSelectorLargeFile(t *testing.T) {
	sel := chunker.DefaultSelector{}
	c := sel.ChunkerFor(600 * 1024 * 1024)
	if c == nil {
		t.Fatal("expected non-nil chunker for large file")
	}
	if _, ok := c.(*chunker.FixedChunker); !ok {
		t.Errorf("expected FixedChunker, got %T", c)
	}
}

func TestImageEngineEmbedsDefaultSelector(t *testing.T) {
	engine := image.NewEngine()
	// Verify it implements ChunkerSelector via embedding
	var cs ChunkerSelector = engine
	c := cs.ChunkerFor(1024)
	if c == nil {
		t.Fatal("expected non-nil chunker from ImageEngine")
	}
}

func TestVideoEngineEmbedsDefaultSelector(t *testing.T) {
	engine := video.NewEngine()
	var cs ChunkerSelector = engine
	c := cs.ChunkerFor(1024)
	if c == nil {
		t.Fatal("expected non-nil chunker from VideoEngine")
	}
}

func TestBinaryEngineEmbedsDefaultSelector(t *testing.T) {
	engine := binary.NewEngine()
	var cs ChunkerSelector = engine
	c := cs.ChunkerFor(1024)
	if c == nil {
		t.Fatal("expected non-nil chunker from BinaryEngine")
	}
}

// TestEngineAutonomy_AllEnginesReturnChunker verifies that each filetype
// engine returns a non-nil chunker for a file size above the whole-file
// threshold. Each engine uses its own tuned chunk sizes (text → 4/8/16 KB;
// binary/image/video → 128/256/512 KB via chunker.fastCDCDefault* aliases).
// The concrete size assertions live in the text and chunker subpackages
// (white-box):
//   - chunker/strategy_test.go TestBinaryChunkerFor_DefaultParams
//
// Engine autonomy is now enforced structurally: ChunkerFor no longer takes
// a cfg parameter, so each engine's private constants are the only source
// of chunk sizes.
func TestEngineAutonomy_AllEnginesReturnChunker(t *testing.T) {
	// Text engine: file size above the 64KB whole-file threshold.
	if c := text.NewEngine().ChunkerFor(1 * 1024 * 1024); c == nil {
		t.Error("text: expected non-nil chunker")
	}
	// Binary / image / video engines.
	if c := binary.NewEngine().ChunkerFor(1 * 1024 * 1024); c == nil {
		t.Error("binary: expected non-nil chunker")
	}
	if c := image.NewEngine().ChunkerFor(1 * 1024 * 1024); c == nil {
		t.Error("image: expected non-nil chunker")
	}
	if c := video.NewEngine().ChunkerFor(1 * 1024 * 1024); c == nil {
		t.Error("video: expected non-nil chunker")
	}
}
