package image

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"  // register GIF decoder for DecodeConfig
	_ "image/jpeg" // register JPEG decoder for DecodeConfig
	_ "image/png"  // register PNG decoder for DecodeConfig
)

// previewFormatName maps canonical format keys to display names.
var previewFormatName = map[string]string{
	formatPNG:  "PNG",
	formatJPEG: "JPEG",
	formatGIF:  "GIF",
	formatWebP: "WebP",
	formatBMP:  "BMP",
	formatTIFF: "TIFF",
}

// Preview returns a one-line summary: format name, dimensions, and
// human-readable file size (e.g. "PNG 1920x1080 2.4 MB").
func (e *ImageEngine) Preview(data []byte, maxLines int) string {
	format := detectFormatByMagic(data)
	name := previewFormatName[format]
	if name == "" {
		name = "image"
	}
	w, h := decodeDimensions(data)
	return fmt.Sprintf("%s %dx%d %s", name, w, h, formatSize(len(data)))
}

// decodeDimensions parses image dimensions from the header using the standard
// library image.DecodeConfig. Returns 0x0 if the format is not decodable.
func decodeDimensions(data []byte) (int, int) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// formatSize returns a human-readable file size string (e.g. "2.4 MB").
func formatSize(n int) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
