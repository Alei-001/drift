package image

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// --- Test helpers for constructing minimal image byte streams ---

// makePNGHeader builds a minimal valid PNG (signature + IHDR chunk) with the
// given dimensions. image.DecodeConfig only needs the header to return dims.
func makePNGHeader(width, height int) []byte {
	// PNG signature
	data := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	// IHDR chunk data: width(4) + height(4) + bitdepth(1) + colortype(1) +
	// compression(1) + filter(1) + interlace(1) = 13 bytes
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = 8  // bit depth
	ihdr[9] = 2  // color type: RGB
	ihdr[10] = 0 // compression method
	ihdr[11] = 0 // filter method
	ihdr[12] = 0 // interlace method
	data = append(data, buildPNGChunk("IHDR", ihdr)...)
	return data
}

// buildPNGChunk assembles a PNG chunk: length(4) + type(4) + data + crc(4).
func buildPNGChunk(chunkType string, chunkData []byte) []byte {
	buf := make([]byte, 4+4+len(chunkData)+4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(chunkData)))
	copy(buf[4:8], chunkType)
	copy(buf[8:8+len(chunkData)], chunkData)
	c := crc32.NewIEEE()
	c.Write([]byte(chunkType))
	c.Write(chunkData)
	binary.BigEndian.PutUint32(buf[8+len(chunkData):], c.Sum32())
	return buf
}

// makeJPEG encodes a real JPEG of the given dimensions using the stdlib.
func makeJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// makeGIF encodes a real GIF of the given dimensions using the stdlib.
func makeGIF(width, height int) []byte {
	img := image.NewPaletted(image.Rect(0, 0, width, height), color.Palette{color.Black})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// --- Detect tests ---

func TestDetect_ByExtension(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		path string
		want bool
	}{
		{"photo.png", true},
		{"photo.jpg", true},
		{"photo.jpeg", true},
		{"animation.gif", true},
		{"modern.webp", true},
		{"legacy.bmp", true},
		{"scan.tiff", true},
		{"scan.tif", true},
		{"readme.txt", false},
		{"source.go", false},
		{"archive.zip", false},
		{"noext", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := engine.DetectByExtension(tc.path); got != tc.want {
				t.Errorf("DetectByExtension(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDetect_ByMagic(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		name   string
		header []byte
		want   bool
	}{
		{"PNG", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, true},
		{"JPEG", []byte{0xFF, 0xD8, 0xFF, 0xE0}, true},
		{"GIF89a", []byte("GIF89a"), true},
		{"GIF87a", []byte("GIF87a"), true},
		{"WebP", append([]byte("RIFF\x00\x00\x00\x00"), []byte("WEBP")...), true},
		{"BMP", []byte{'B', 'M', 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00}, true},
		{"TIFF little-endian", []byte{'I', 'I', 0x2A, 0x00}, true},
		{"TIFF big-endian", []byte{'M', 'M', 0x00, 0x2A}, true},
		{"unknown", []byte("plain text"), false},
		{"empty", []byte{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.DetectByMagic(tc.header); got != tc.want {
				t.Errorf("DetectByMagic %s = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestDetect_ByHeuristic(t *testing.T) {
	engine := NewEngine()
	// Images are never sniffed heuristically; this layer always returns false.
	tests := []struct {
		name   string
		path   string
		header []byte
	}{
		{"image magic bytes", "unknown.bin", []byte{0x89, 'P', 'N', 'G'}},
		{"plain text", "noext", []byte("hello world")},
		{"empty", "empty.dat", []byte{}},
		{"image extension", "photo.png", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.DetectByHeuristic(tc.path, tc.header); got != false {
				t.Errorf("DetectByHeuristic(%q, ...) = %v, want false", tc.name, got)
			}
		})
	}
}

// --- Diff tests ---

func TestDiff_FormatChanged(t *testing.T) {
	engine := NewEngine()
	oldData := makePNGHeader(10, 10)
	newData := makeJPEG(10, 10)

	diff, err := engine.Diff(context.Background(), "old.png", bytes.NewReader(oldData), "new.jpg", bytes.NewReader(newData))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	expected := "image format changed: png -> jpg"
	if diff != expected {
		t.Errorf("expected %q, got %q", expected, diff)
	}
}

func TestDiff_DimensionsChanged(t *testing.T) {
	engine := NewEngine()
	oldData := makePNGHeader(1920, 1080)
	newData := makePNGHeader(3840, 2160)

	diff, err := engine.Diff(context.Background(), "old.png", bytes.NewReader(oldData), "new.png", bytes.NewReader(newData))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	expected := "image dimensions changed: 1920x1080 -> 3840x2160"
	if diff != expected {
		t.Errorf("expected %q, got %q", expected, diff)
	}
}

func TestDiff_FileSizeChanged(t *testing.T) {
	engine := NewEngine()
	// Two PNGs with identical dimensions but different total byte counts.
	oldData := makePNGHeader(4, 4)
	newData := append(append([]byte{}, oldData...), 0x00, 0x00, 0x00)

	diff, err := engine.Diff(context.Background(), "old.png", bytes.NewReader(oldData), "new.png", bytes.NewReader(newData))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.HasPrefix(diff, "image file size changed:") {
		t.Errorf("expected file size change message, got %q", diff)
	}
}

func TestDiff_Identical(t *testing.T) {
	engine := NewEngine()
	data := makePNGHeader(8, 8)

	diff, err := engine.Diff(context.Background(), "old.png", bytes.NewReader(data), "new.png", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for identical data, got %q", diff)
	}
}

// --- Preview tests ---

func TestPreview_PNG(t *testing.T) {
	engine := NewEngine()
	data := makePNGHeader(640, 480)
	preview, err := engine.Preview(data, int64(len(data)), nil, 10)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if !strings.Contains(preview, "PNG") {
		t.Errorf("preview should contain format name 'PNG', got %q", preview)
	}
	if !strings.Contains(preview, "640x480") {
		t.Errorf("preview should contain dimensions '640x480', got %q", preview)
	}
	if !strings.Contains(preview, "B") {
		t.Errorf("preview should contain a file size, got %q", preview)
	}
}

func TestPreview_JPEG(t *testing.T) {
	engine := NewEngine()
	data := makeJPEG(2, 2)
	preview, err := engine.Preview(data, int64(len(data)), nil, 10)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if !strings.Contains(preview, "JPEG") {
		t.Errorf("preview should contain 'JPEG', got %q", preview)
	}
	if !strings.Contains(preview, "2x2") {
		t.Errorf("preview should contain '2x2', got %q", preview)
	}
}

func TestPreview_GIF(t *testing.T) {
	engine := NewEngine()
	data := makeGIF(3, 3)
	preview, err := engine.Preview(data, int64(len(data)), nil, 10)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if !strings.Contains(preview, "GIF") {
		t.Errorf("preview should contain 'GIF', got %q", preview)
	}
	if !strings.Contains(preview, "3x3") {
		t.Errorf("preview should contain '3x3', got %q", preview)
	}
}

func TestPreview_UnknownFormat(t *testing.T) {
	engine := NewEngine()
	// Magic bytes for BMP, but not decodable by image.DecodeConfig; should
	// still produce a preview line with format name and size.
	data := []byte{'B', 'M', 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x28, 0x00, 0x00, 0x00}
	preview, err := engine.Preview(data, int64(len(data)), nil, 10)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}

	if !strings.Contains(preview, "BMP") {
		t.Errorf("preview should contain 'BMP', got %q", preview)
	}
}

// --- Name and NewEngine ---

func TestName(t *testing.T) {
	engine := NewEngine()
	if got := engine.Name(); got != "image" {
		t.Errorf("Name() = %q, want %q", got, "image")
	}
}

// --- ChunkerFor smoke test ---

func TestChunkerFor(t *testing.T) {
	engine := NewEngine()
	// 200KB — below 50MB, expects a non-nil default FastCDC chunker.
	c := engine.ChunkerFor(200 * 1024)
	if c == nil {
		t.Fatal("expected non-nil chunker for 200KB image, got nil")
	}
	// Verify the chunker actually chunks data end-to-end.
	data := bytes.Repeat([]byte("image-bytes-pattern-"), 20000)
	chunks, err := c.Chunk(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk, got none")
	}
	var total uint32
	for _, ch := range chunks {
		total += ch.Size
	}
	if int(total) != len(data) {
		t.Errorf("total chunk size %d != original %d", total, len(data))
	}
}

// Ensure png and jpeg stdlib packages are referenced so the imports are not
// removed by tooling even though the blank imports in preview.go handle the
// decoder registration.
var (
	_ = png.Encode
	_ = jpeg.Encode
)
