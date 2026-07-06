package filetype

import (
	"testing"

	"github.com/your-org/drift/internal/chunker"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype/binary"
	"github.com/your-org/drift/internal/filetype/image"
	"github.com/your-org/drift/internal/filetype/video"
)

func TestDefaultSelectorSmallFile(t *testing.T) {
	sel := chunker.DefaultSelector{}
	c := sel.ChunkerFor(1024, &core.DefaultConfig().Core)
	if c == nil {
		t.Fatal("expected non-nil chunker for small file")
	}
	if _, ok := c.(*chunker.FastCDCChunker); !ok {
		t.Errorf("expected FastCDCChunker, got %T", c)
	}
}

func TestDefaultSelectorLargeFile(t *testing.T) {
	sel := chunker.DefaultSelector{}
	c := sel.ChunkerFor(600*1024*1024, &core.DefaultConfig().Core)
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
	c := cs.ChunkerFor(1024, &core.DefaultConfig().Core)
	if c == nil {
		t.Fatal("expected non-nil chunker from ImageEngine")
	}
}

func TestVideoEngineEmbedsDefaultSelector(t *testing.T) {
	engine := video.NewEngine()
	var cs ChunkerSelector = engine
	c := cs.ChunkerFor(1024, &core.DefaultConfig().Core)
	if c == nil {
		t.Fatal("expected non-nil chunker from VideoEngine")
	}
}

func TestBinaryEngineEmbedsDefaultSelector(t *testing.T) {
	engine := binary.NewEngine()
	var cs ChunkerSelector = engine
	c := cs.ChunkerFor(1024, &core.DefaultConfig().Core)
	if c == nil {
		t.Fatal("expected non-nil chunker from BinaryEngine")
	}
}
