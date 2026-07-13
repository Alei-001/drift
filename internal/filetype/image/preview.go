package image

import (
	"fmt"
	"io"

	sizefmt "github.com/Alei-001/drift/internal/util/format"
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
//
// Only the header is inspected (for magic detection and dimension parsing)
// and size is taken from the caller; the content reader is never touched.
// This keeps memory constant for arbitrarily large images. Dimension
// parsing delegates to format.DecodeDimensions so that PNG, JPEG, GIF,
// WebP, BMP, and TIFF are all supported.
func (e *ImageEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
	_ = reader
	_ = maxLines
	format := detectFormatByMagic(header)
	name := previewFormatName[format]
	if name == "" {
		name = "image"
	}
	w, h := sizefmt.DecodeDimensions(header)
	return fmt.Sprintf("%s %dx%d %s", name, w, h, sizefmt.Bytes(int64(size))), nil
}
