package image

import (
	"encoding/binary"
	"path/filepath"
	"strings"

	"github.com/your-org/drift/chunker"
)

// Canonical image format keys returned by detectFormatByMagic.
const (
	formatPNG  = "png"
	formatJPEG = "jpg"
	formatGIF  = "gif"
	formatWebP = "webp"
	formatBMP  = "bmp"
	formatTIFF = "tiff"
)

var imageExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true, ".tiff": true, ".tif": true,
}

// ImageEngine handles image files (PNG, JPEG, GIF, WebP, BMP, TIFF).
type ImageEngine struct {
	chunker.DefaultSelector
}

// NewEngine creates a new ImageEngine.
func NewEngine() *ImageEngine {
	return &ImageEngine{}
}

// Name returns "image".
func (e *ImageEngine) Name() string {
	return "image"
}

// DetectByMagic checks file header signatures for known image magic bytes.
func (e *ImageEngine) DetectByMagic(header []byte) bool {
	return detectFormatByMagic(header) != ""
}

// DetectByExtension checks if the path's extension is a known image type.
func (e *ImageEngine) DetectByExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return imageExtensions[ext]
}

// DetectByHeuristic returns false; images are not sniffed heuristically.
func (e *ImageEngine) DetectByHeuristic(path string, header []byte) bool {
	return false
}

// detectFormatByMagic returns the canonical format key if the header matches
// a known image magic byte sequence, or "" if unknown.
func detectFormatByMagic(header []byte) string {
	if len(header) == 0 {
		return ""
	}
	// PNG: \x89PNG\r\n\x1a\n
	if len(header) >= 4 && header[0] == 0x89 && header[1] == 'P' && header[2] == 'N' && header[3] == 'G' {
		return formatPNG
	}
	// JPEG: \xFF\xD8\xFF
	if len(header) >= 3 && header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return formatJPEG
	}
	// GIF: GIF8 (covers GIF87a and GIF89a)
	if len(header) >= 4 && string(header[:4]) == "GIF8" {
		return formatGIF
	}
	// WebP: RIFF....WEBP
	if len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
		return formatWebP
	}
	// BMP: "BM" magic plus a valid DIB header size at offset 14-17.
	// Requiring the DIB size avoids false positives on text files that
	// happen to start with "BM" (e.g. "BMW parts..."). Known valid DIB
	// header sizes: 12 (BITMAPCOREHEADER), 40 (BITMAPINFOHEADER), 52, 56,
	// 64, 108 (BITMAPV4HEADER), 124 (BITMAPV5HEADER).
	if len(header) >= 18 && header[0] == 'B' && header[1] == 'M' {
		dibSize := binary.LittleEndian.Uint32(header[14:18])
		switch dibSize {
		case 12, 40, 52, 56, 64, 108, 124:
			return formatBMP
		}
	}
	// TIFF: II*\x00 (little-endian) or MM\x00* (big-endian)
	if len(header) >= 4 {
		if header[0] == 'I' && header[1] == 'I' && header[2] == 0x2A && header[3] == 0x00 {
			return formatTIFF
		}
		if header[0] == 'M' && header[1] == 'M' && header[2] == 0x00 && header[3] == 0x2A {
			return formatTIFF
		}
	}
	return ""
}
