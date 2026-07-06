package format

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for ImageDimensions
	_ "image/jpeg" // register JPEG decoder for ImageDimensions
	_ "image/png"  // register PNG decoder for ImageDimensions
)

const (
	sizeKB = 1024
	sizeMB = 1024 * 1024
	sizeGB = 1024 * 1024 * 1024
)

// Bytes returns a human-readable size string (e.g. "2.5 MB").
// Negative sizes are formatted with a leading minus sign.
func Bytes(size int64) string {
	if size < 0 {
		mag := uint64(-(size + 1)) + 1 // safe abs for MinInt64
		return "-" + bytesPositive(mag)
	}
	return bytesPositive(uint64(size))
}

// bytesPositive formats a non-negative magnitude as a human-readable size
// string using 1024-based units (B, KB, MB, GB).
func bytesPositive(size uint64) string {
	switch {
	case size < sizeKB:
		return fmt.Sprintf("%d B", size)
	case size < sizeMB:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	case size < sizeGB:
		return fmt.Sprintf("%.1f MB", float64(size)/sizeMB)
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/sizeGB)
	}
}

// ImageDimensions decodes image dimensions from data for common image formats
// (PNG, JPEG, GIF). Returns empty string for non-image or undecodable data.
func ImageDimensions(data []byte) string {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%dx%d", cfg.Width, cfg.Height)
}
