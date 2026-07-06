package format

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestBytes_Units(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want string
	}{
		{"zero", 0, "0 B"},
		{"one byte", 1, "1 B"},
		{"under 1KB", 512, "512 B"},
		{"exactly 1KB", 1024, "1.0 KB"},
		{"under 1MB", 1024 * 512, "512.0 KB"},
		{"exactly 1MB", 1024 * 1024, "1.0 MB"},
		{"under 1GB", 1024 * 1024 * 512, "512.0 MB"},
		{"exactly 1GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"multiple GB", 2 * 1024 * 1024 * 1024, "2.0 GB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Bytes(tc.size)
			if got != tc.want {
				t.Errorf("Bytes(%d) = %q, want %q", tc.size, got, tc.want)
			}
		})
	}
}

func TestBytes_Negative(t *testing.T) {
	got := Bytes(-1024)
	// Negative sizes are formatted with a leading minus sign.
	if !strings.HasPrefix(got, "-") {
		t.Errorf("Bytes(-1024) = %q, want leading '-'", got)
	}
	if !strings.HasSuffix(got, "KB") {
		t.Errorf("Bytes(-1024) = %q, want KB suffix", got)
	}
}

func TestBytes_Boundaries(t *testing.T) {
	// 1023 B is still in bytes range
	if got := Bytes(1023); got != "1023 B" {
		t.Errorf("Bytes(1023) = %q, want %q", got, "1023 B")
	}
	// 1024 B crosses into KB
	if got := Bytes(1024); got != "1.0 KB" {
		t.Errorf("Bytes(1024) = %q, want %q", got, "1.0 KB")
	}
	// 1024*1024 - 1 is still KB
	if got := Bytes(1024*1024 - 1); !strings.HasSuffix(got, "KB") {
		t.Errorf("Bytes(1024*1024-1) = %q, want KB suffix", got)
	}
}

func TestImageDimensions_Invalid(t *testing.T) {
	// Empty / non-image data returns empty string.
	if got := ImageDimensions(nil); got != "" {
		t.Errorf("ImageDimensions(nil) = %q, want empty", got)
	}
	if got := ImageDimensions([]byte("not an image")); got != "" {
		t.Errorf("ImageDimensions(text) = %q, want empty", got)
	}
	if got := ImageDimensions([]byte{}); got != "" {
		t.Errorf("ImageDimensions(empty) = %q, want empty", got)
	}
}

func TestImageDimensions_ValidPNG(t *testing.T) {
	// Generate a valid 4x2 PNG using the standard library.
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode failed: %v", err)
	}
	got := ImageDimensions(buf.Bytes())
	if got != "4x2" {
		t.Errorf("ImageDimensions(png) = %q, want %q", got, "4x2")
	}
}
