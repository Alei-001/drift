package porcelain

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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

// fastCDCParams reads the unexported min/avg/max fields of a FastCDCChunker
// via reflection so tests can verify which configuration was selected.
func fastCDCParams(t *testing.T, c *chunker.FastCDCChunker) (min, avg, max int) {
	t.Helper()
	v := reflect.ValueOf(c).Elem()
	min = int(v.FieldByName("minSize").Int())
	avg = int(v.FieldByName("avgSize").Int())
	max = int(v.FieldByName("maxSize").Int())
	return
}

// fixedChunkSize reads the unexported chunkSize field of a FixedChunker.
func fixedChunkSize(t *testing.T, c *chunker.FixedChunker) int {
	t.Helper()
	v := reflect.ValueOf(c).Elem()
	return int(v.FieldByName("chunkSize").Int())
}

// binaryClassEngines returns the engines that share the binary-class 3-tier
// strategy, for table-driven verification.
func binaryClassEngines() map[string]filetype.Engine {
	return map[string]filetype.Engine{
		"image":  image.NewEngine(),
		"video":  video.NewEngine(),
		"binary": binary.NewEngine(),
	}
}

func TestChunkerFor_TextSmall(t *testing.T) {
	// 10KB text file — below the 64KB whole-file threshold.
	eng := text.NewEngine()
	c := eng.ChunkerFor(10 * 1024)
	if c != nil {
		t.Errorf("expected nil (whole-file) for 10KB text, got %T", c)
	}
}

func TestChunkerFor_TextMedium(t *testing.T) {
	// 1MB text file — between 64KB and 50MB, expects FastCDC(4K/8K/16K).
	eng := text.NewEngine()
	c := eng.ChunkerFor(1 * 1024 * 1024)
	fc, ok := c.(*chunker.FastCDCChunker)
	if !ok {
		t.Fatalf("expected *FastCDCChunker for 1MB text, got %T", c)
	}
	min, avg, max := fastCDCParams(t, fc)
	if min != 4096 || avg != 8192 || max != 16384 {
		t.Errorf("text medium: expected 4K/8K/16K, got %d/%d/%d", min, avg, max)
	}
}

func TestChunkerFor_BinarySmall(t *testing.T) {
	// 10MB binary/image/video file — below 50MB, expects default FastCDC.
	for name, eng := range binaryClassEngines() {
		c := eng.ChunkerFor(10 * 1024 * 1024)
		fc, ok := c.(*chunker.FastCDCChunker)
		if !ok {
			t.Errorf("%s 10MB: expected *FastCDCChunker, got %T", name, c)
			continue
		}
		min, avg, max := fastCDCParams(t, fc)
		if min != 128*1024 || avg != 256*1024 || max != 512*1024 {
			t.Errorf("%s 10MB: expected default 128K/256K/512K, got %d/%d/%d", name, min, avg, max)
		}
	}
}

func TestChunkerFor_BinaryLarge(t *testing.T) {
	// 100MB — between 50MB and 500MB, expects FastCDC(1M/2M/4M).
	for name, eng := range binaryClassEngines() {
		c := eng.ChunkerFor(100 * 1024 * 1024)
		fc, ok := c.(*chunker.FastCDCChunker)
		if !ok {
			t.Errorf("%s 100MB: expected *FastCDCChunker, got %T", name, c)
			continue
		}
		min, avg, max := fastCDCParams(t, fc)
		if min != 1048576 || avg != 2097152 || max != 4194304 {
			t.Errorf("%s 100MB: expected 1M/2M/4M, got %d/%d/%d", name, min, avg, max)
		}
	}
}

func TestChunkerFor_BinaryHuge(t *testing.T) {
	// 600MB — at or above 500MB, expects Fixed(8MB).
	for name, eng := range binaryClassEngines() {
		c := eng.ChunkerFor(600 * 1024 * 1024)
		fc, ok := c.(*chunker.FixedChunker)
		if !ok {
			t.Errorf("%s 600MB: expected *FixedChunker, got %T", name, c)
			continue
		}
		if sz := fixedChunkSize(t, fc); sz != 8*1024*1024 {
			t.Errorf("%s 600MB: expected 8MB fixed, got %d", name, sz)
		}
	}
}

func TestChunkerFor_BoundaryValues(t *testing.T) {
	textEng := text.NewEngine()
	binEng := binary.NewEngine()

	// 64KB exactly — text crosses into FastCDC territory (>= 64KB).
	if c := textEng.ChunkerFor(64 * 1024); c == nil {
		t.Error("64KB text: expected non-nil FastCDC, got nil")
	}

	// 50MB exactly — binary crosses into the large-file tier (>= 50MB).
	c := binEng.ChunkerFor(50 * 1024 * 1024)
	fc, ok := c.(*chunker.FastCDCChunker)
	if !ok {
		t.Fatalf("50MB binary: expected *FastCDCChunker, got %T", c)
	}
	min, avg, max := fastCDCParams(t, fc)
	if min != 1048576 || avg != 2097152 || max != 4194304 {
		t.Errorf("50MB binary: expected 1M/2M/4M, got %d/%d/%d", min, avg, max)
	}

	// 500MB exactly — binary crosses into the fixed-chunk tier (>= 500MB).
	c = binEng.ChunkerFor(500 * 1024 * 1024)
	if _, ok := c.(*chunker.FixedChunker); !ok {
		t.Errorf("500MB binary: expected *FixedChunker, got %T", c)
	}

	// Just below the text threshold (64KB - 1) should be nil (whole-file).
	if c := textEng.ChunkerFor(64*1024 - 1); c != nil {
		t.Errorf("64KB-1 text: expected nil, got %T", c)
	}

	// Just below 50MB binary should still use default FastCDC.
	c = binEng.ChunkerFor(50*1024*1024 - 1)
	fc, ok = c.(*chunker.FastCDCChunker)
	if !ok {
		t.Errorf("50MB-1 binary: expected *FastCDCChunker, got %T", c)
	}
	if ok {
		min, avg, max = fastCDCParams(t, fc)
		if min != 128*1024 || avg != 256*1024 || max != 512*1024 {
			t.Errorf("50MB-1 binary: expected default 128K/256K/512K, got %d/%d/%d", min, avg, max)
		}
	}

	// Just below 500MB binary should still use large FastCDC.
	c = binEng.ChunkerFor(500*1024*1024 - 1)
	fc, ok = c.(*chunker.FastCDCChunker)
	if !ok {
		t.Errorf("500MB-1 binary: expected *FastCDCChunker, got %T", c)
	}
}

// --- Consistency between CreateSnapshot and ComputeFileHash ---

// newTestStore creates a memory store with HEAD/index set up for snapshots.
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

// consistencyCase holds the parameters for one consistency sub-test.
type consistencyCase struct {
	name      string
	filename  string
	content   []byte
	engineHit string // expected detected engine name, for diagnostics
}

func TestComputeFileHash_ConsistencyWithCreateSnapshot(t *testing.T) {
	// Build three files covering the three target tiers:
	//  1. text small  (< 64KB)       -> whole-file single chunk
	//  2. text medium (64KB..50MB)   -> FastCDC(4K/8K/16K)
	//  3. binary small (< 50MB)      -> default FastCDC (image/video share this tier)
	smallText := make([]byte, 1024) // 1KB, well under 64KB
	for i := range smallText {
		smallText[i] = 'a' + byte(i%26)
	}

	mediumText := make([]byte, 200*1024) // 200KB, between 64KB and 50MB
	for i := range mediumText {
		mediumText[i] = 'A' + byte(i%26)
	}

	// Binary content containing a null byte so the text engine rejects it
	// and the binary fallback engine is selected (same tier as image/video).
	binarySmall := make([]byte, 1024*1024) // 1MB
	for i := range binarySmall {
		binarySmall[i] = byte(i % 256)
	}
	binarySmall[0] = 0x00 // null byte -> detected as binary

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

			// CreateSnapshot stores the file hash in the snapshot entry.
			snap, err := CreateSnapshot(context.Background(), store, dir, "test "+tc.name, "test", nil)
			if err != nil {
				t.Fatalf("CreateSnapshot failed: %v", err)
			}

			// Locate the entry for our file.
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

			// ComputeFileHash must return the same hash.
			computedHash, err := ComputeFileHash(fullPath)
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
