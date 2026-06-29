package image

import (
	"bytes"
	"fmt"
)

// Diff compares two images by format, dimensions, and file size, in that
// priority order. It does not perform pixel-level diffing. Returns an empty
// string when the images are identical.
func (e *ImageEngine) Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error) {
	// Short-circuit: identical bytes mean no change.
	if bytes.Equal(oldData, newData) {
		return "", nil
	}

	oldFormat := detectFormatByMagic(oldData)
	newFormat := detectFormatByMagic(newData)
	if oldFormat != newFormat {
		return fmt.Sprintf("image format changed: %s -> %s",
			formatKeyOrUnknown(oldFormat), formatKeyOrUnknown(newFormat)), nil
	}

	oldW, oldH := decodeDimensions(oldData)
	newW, newH := decodeDimensions(newData)
	if oldW != newW || oldH != newH {
		return fmt.Sprintf("image dimensions changed: %dx%d -> %dx%d",
			oldW, oldH, newW, newH), nil
	}

	if len(oldData) != len(newData) {
		return fmt.Sprintf("image file size changed: %s -> %s",
			formatSize(len(oldData)), formatSize(len(newData))), nil
	}

	return "", nil
}

// formatKeyOrUnknown returns the format key or "unknown" for empty keys.
func formatKeyOrUnknown(f string) string {
	if f == "" {
		return "unknown"
	}
	return f
}
