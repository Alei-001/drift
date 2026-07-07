package binary

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/chunker"
)

// --- Name and NewEngine ---

func TestName(t *testing.T) {
	engine := NewEngine()
	if got := engine.Name(); got != "binary" {
		t.Errorf("Name() = %q, want %q", got, "binary")
	}
}

// --- Detection ---

// TestDetect_ByMagic verifies that the binary engine claims no magic bytes.
// It is the fallback engine and must never win the magic-byte layer.
func TestDetect_ByMagic(t *testing.T) {
	engine := NewEngine()
	cases := [][]byte{
		nil,
		{},
		{0x89, 'P', 'N', 'G'},
		{0xFF, 0xD8, 0xFF},
		{0x00, 0x01, 0x02, 0x03},
	}
	for i, header := range cases {
		if got := engine.DetectByMagic(header); got != false {
			t.Errorf("case %d: DetectByMagic(% x) = %v, want false", i, header, got)
		}
	}
}

// TestDetect_ByExtension verifies that the binary engine claims no extension.
// It is the fallback engine and must never win the extension layer.
func TestDetect_ByExtension(t *testing.T) {
	engine := NewEngine()
	cases := []string{
		"",
		"file.bin",
		"file.dat",
		"image.png",
		"video.mp4",
		"unknown.unknown",
	}
	for _, path := range cases {
		if got := engine.DetectByExtension(path); got != false {
			t.Errorf("DetectByExtension(%q) = %v, want false", path, got)
		}
	}
}

// TestDetect_ByHeuristic verifies that the binary engine matches anything.
// This is the contract that makes it the universal fallback: when no other
// engine's magic or extension matched, the registry's heuristic layer reaches
// the binary engine and it claims the file.
func TestDetect_ByHeuristic(t *testing.T) {
	engine := NewEngine()
	cases := []struct {
		path   string
		header []byte
	}{
		{"", nil},
		{"anything.bin", []byte{0x00, 0x01}},
		{"no-extension", []byte("hello")},
		{"image.png", []byte{0x89, 'P', 'N', 'G'}},
		{"empty", []byte{}},
	}
	for _, tc := range cases {
		if got := engine.DetectByHeuristic(tc.path, tc.header); got != true {
			t.Errorf("DetectByHeuristic(%q, % x) = %v, want true", tc.path, tc.header, got)
		}
	}
}

// --- ChunkerFor ---

// TestChunkerFor_SmallFile verifies the FastCDC path for files in the
// 0–50MB band. The binary engine delegates to chunker.DefaultSelector, which
// selects FastCDC for this size range.
func TestChunkerFor_SmallFile(t *testing.T) {
	engine := NewEngine()
	c := engine.ChunkerFor(10 * 1024 * 1024)
	if c == nil {
		t.Fatal("expected non-nil chunker for 10MB binary file")
	}
	if _, ok := c.(*chunker.FastCDCChunker); !ok {
		t.Errorf("expected *FastCDCChunker for 10MB, got %T", c)
	}
}

// TestChunkerFor_LargeFile verifies the FixedChunker path for files >= 500MB.
// Large files use fixed-size chunking to keep chunk count bounded.
func TestChunkerFor_LargeFile(t *testing.T) {
	engine := NewEngine()
	c := engine.ChunkerFor(600 * 1024 * 1024)
	if c == nil {
		t.Fatal("expected non-nil chunker for 600MB binary file")
	}
	if _, ok := c.(*chunker.FixedChunker); !ok {
		t.Errorf("expected *FixedChunker for 600MB, got %T", c)
	}
}

// --- Diff ---

func TestDiff_Identical(t *testing.T) {
	engine := NewEngine()
	data := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}

	diff, err := engine.Diff(context.Background(), "old.bin", bytes.NewReader(data), "new.bin", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for identical binary content, got %q", diff)
	}
}

func TestDiff_Different(t *testing.T) {
	engine := NewEngine()
	oldData := []byte{0x00, 0x01, 0x02, 0x03}
	newData := []byte{0x00, 0x01, 0x02, 0x04}

	diff, err := engine.Diff(context.Background(), "old.bin", bytes.NewReader(oldData), "new.bin", bytes.NewReader(newData))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "binary files differ" {
		t.Errorf("expected 'binary files differ', got %q", diff)
	}
}

func TestDiff_BothEmpty(t *testing.T) {
	engine := NewEngine()

	diff, err := engine.Diff(context.Background(), "old.bin", bytes.NewReader([]byte{}), "new.bin", bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for both empty, got %q", diff)
	}
}

func TestDiff_OneEmpty(t *testing.T) {
	engine := NewEngine()

	diff, err := engine.Diff(context.Background(), "old.bin", bytes.NewReader([]byte{}), "new.bin", bytes.NewReader([]byte{0x01}))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "binary files differ" {
		t.Errorf("expected 'binary files differ', got %q", diff)
	}
}

func TestDiff_OldReaderError(t *testing.T) {
	engine := NewEngine()
	errReader := &errReader{err: io.ErrUnexpectedEOF}

	_, err := engine.Diff(context.Background(), "old.bin", errReader, "new.bin", bytes.NewReader([]byte{0x01}))
	if err == nil {
		t.Fatal("expected error from broken reader, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("expected 'unexpected EOF' in error, got %v", err)
	}
}

func TestDiff_NewReaderError(t *testing.T) {
	engine := NewEngine()
	errReader := &errReader{err: io.ErrUnexpectedEOF}

	_, err := engine.Diff(context.Background(), "old.bin", bytes.NewReader([]byte{0x01}), "new.bin", errReader)
	if err == nil {
		t.Fatal("expected error from broken reader, got nil")
	}
}

// errReader is a Reader that always fails with the given error on Read.
type errReader struct{ err error }

func (r *errReader) Read(p []byte) (int, error) { return 0, r.err }

// --- Metadata ---

func TestMetadata(t *testing.T) {
	engine := NewEngine()
	md := engine.Metadata()
	if md == nil {
		t.Fatal("expected non-nil metadata")
	}
	if md.MIMEType != "application/octet-stream" {
		t.Errorf("expected MIMEType 'application/octet-stream', got %q", md.MIMEType)
	}
}

// --- Preview ---

func TestPreview(t *testing.T) {
	engine := NewEngine()
	cases := []struct {
		name     string
		header   []byte
		size     int64
		maxLines int
	}{
		{"empty", nil, 0, 0},
		{"small", []byte{0x00, 0x01}, 100, 5},
		{"large", []byte{0xFF}, 1 << 30, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := engine.Preview(tc.header, tc.size, bytes.NewReader(tc.header), tc.maxLines)
			if err != nil {
				t.Fatalf("Preview failed: %v", err)
			}
			if got != "[binary file]" {
				t.Errorf("expected '[binary file]', got %q", got)
			}
		})
	}
}

// TestPreview_NilReader verifies that Preview does not read from the reader
// (binary previews are constant). Passing a nil reader must not panic.
func TestPreview_NilReader(t *testing.T) {
	engine := NewEngine()
	got, err := engine.Preview(nil, 1024, nil, 10)
	if err != nil {
		t.Fatalf("Preview with nil reader failed: %v", err)
	}
	if got != "[binary file]" {
		t.Errorf("expected '[binary file]', got %q", got)
	}
}
