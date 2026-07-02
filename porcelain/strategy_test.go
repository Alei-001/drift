package porcelain

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/your-org/drift/chunker"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/filetype/binary"
	"github.com/your-org/drift/filetype/image"
	"github.com/your-org/drift/filetype/text"
	"github.com/your-org/drift/filetype/video"
	"github.com/your-org/drift/storage/memory"
)

func binaryClassEngines() map[string]filetype.Engine {
	return map[string]filetype.Engine{
		"image":  image.NewEngine(),
		"video":  video.NewEngine(),
		"binary": binary.NewEngine(),
	}
}

func TestChunkerFor_TextSmall(t *testing.T) {
	eng := text.NewEngine()
	c := eng.ChunkerFor(10*1024, nil)
	if c != nil {
		t.Errorf("expected nil (whole-file) for 10KB text, got %T", c)
	}
}

func TestChunkerFor_TextMedium(t *testing.T) {
	eng := text.NewEngine()
	c := eng.ChunkerFor(1*1024*1024, nil)
	if c == nil {
		t.Fatal("expected non-nil chunker for 1MB text, got nil")
	}
	chunks, err := c.Chunk(bytes.NewReader(make([]byte, 100*1024)))
	if err != nil {
		t.Fatalf("chunking failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
}

func TestChunkerFor_BinarySmall(t *testing.T) {
	for name, eng := range binaryClassEngines() {
		c := eng.ChunkerFor(10*1024*1024, nil)
		if _, ok := c.(*chunker.FastCDCChunker); !ok {
			t.Errorf("%s 10MB: expected *FastCDCChunker, got %T", name, c)
		}
	}
}

func TestChunkerFor_BinaryLarge(t *testing.T) {
	for name, eng := range binaryClassEngines() {
		c := eng.ChunkerFor(100*1024*1024, nil)
		if _, ok := c.(*chunker.FastCDCChunker); !ok {
			t.Errorf("%s 100MB: expected *FastCDCChunker, got %T", name, c)
		}
	}
}

func TestChunkerFor_BinaryHuge(t *testing.T) {
	for name, eng := range binaryClassEngines() {
		c := eng.ChunkerFor(600*1024*1024, nil)
		if _, ok := c.(*chunker.FixedChunker); !ok {
			t.Errorf("%s 600MB: expected *FixedChunker, got %T", name, c)
		}
	}
}

func TestChunkerFor_BoundaryValues(t *testing.T) {
	textEng := text.NewEngine()
	binEng := binary.NewEngine()

	if c := textEng.ChunkerFor(64*1024, nil); c == nil {
		t.Error("64KB text: expected non-nil FastCDC, got nil")
	}

	c := binEng.ChunkerFor(50*1024*1024, nil)
	if _, ok := c.(*chunker.FastCDCChunker); !ok {
		t.Errorf("50MB binary: expected *FastCDCChunker, got %T", c)
	}

	c = binEng.ChunkerFor(500*1024*1024, nil)
	if _, ok := c.(*chunker.FixedChunker); !ok {
		t.Errorf("500MB binary: expected *FixedChunker, got %T", c)
	}

	if c := textEng.ChunkerFor(64*1024-1, nil); c != nil {
		t.Errorf("64KB-1 text: expected nil, got %T", c)
	}

	c = binEng.ChunkerFor(50*1024*1024-1, nil)
	if _, ok := c.(*chunker.FastCDCChunker); !ok {
		t.Errorf("50MB-1 binary: expected *FastCDCChunker, got %T", c)
	}

	c = binEng.ChunkerFor(500*1024*1024-1, nil)
	if _, ok := c.(*chunker.FastCDCChunker); !ok {
		t.Errorf("500MB-1 binary: expected *FastCDCChunker, got %T", c)
	}
}

func newTestStore(t *testing.T) *memory.MemoryStorage {
	t.Helper()
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})
	return store
}

type consistencyCase struct {
	name      string
	filename  string
	content   []byte
	engineHit string
}

func TestComputeFileHash_ConsistencyWithCreateSnapshot(t *testing.T) {
	smallText := make([]byte, 1024)
	for i := range smallText {
		smallText[i] = 'a' + byte(i%26)
	}

	mediumText := make([]byte, 200*1024)
	for i := range mediumText {
		mediumText[i] = 'A' + byte(i%26)
	}

	binarySmall := make([]byte, 1024*1024)
	for i := range binarySmall {
		binarySmall[i] = byte(i % 256)
	}
	binarySmall[0] = 0x00

	cases := []consistencyCase{
		{name: "text_small", filename: "small.txt", content: smallText, engineHit: "text"},
		{name: "text_medium", filename: "medium.txt", content: mediumText, engineHit: "text"},
		{name: "binary_small", filename: "data.bin", content: binarySmall, engineHit: "binary"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)
			dir := t.TempDir()
			fullPath := filepath.Join(dir, tc.filename)
			if err := os.WriteFile(fullPath, tc.content, 0644); err != nil {
				t.Fatalf("write file: %v", err)
			}

			snap, err := CreateSnapshot(context.Background(), store, dir, "test "+tc.name, "test", nil, nil)
			if err != nil {
				t.Fatalf("CreateSnapshot failed: %v", err)
			}

			var snapHash core.Hash
			found := false
			for _, fe := range snap.Files {
				if fe.Path == tc.filename {
					snapHash = fe.Hash
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("file %s not found in snapshot entries", tc.filename)
			}

			computedHash, err := ComputeFileHash(fullPath, nil)
			if err != nil {
				t.Fatalf("ComputeFileHash failed: %v", err)
			}
			if computedHash != snapHash {
				t.Errorf("hash mismatch: CreateSnapshot=%s, ComputeFileHash=%s",
					snapHash.FullString(), computedHash.FullString())
			}
		})
	}
}
