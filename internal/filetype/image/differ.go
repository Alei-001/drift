package image

import (
	"bytes"
	"fmt"
	"io"

	"github.com/your-org/drift/internal/util/format"
)

// Diff compares two images by format, dimensions, and file size, in that
// priority order. It does not perform pixel-level diffing. Returns an empty
// string when the images are identical.
//
// The full bytes are needed for equality and size comparison, so both
// readers are consumed entirely. Image diffs in drift are only invoked for
// explicitly diffed files (not bulk snapshot diffs), so this is acceptable.
func (e *ImageEngine) Diff(oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldData, err := io.ReadAll(oldReader)
	if err != nil {
		return "", fmt.Errorf("read old image %s: %w", oldPath, err)
	}
	newData, err := io.ReadAll(newReader)
	if err != nil {
		return "", fmt.Errorf("read new image %s: %w", newPath, err)
	}

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
			format.Bytes(int64(len(oldData))), format.Bytes(int64(len(newData)))), nil
	}

	// Fallback: same format, dimensions, and byte length, but the bytes
	// differ (checked at the top). Report a generic content change so we
	// don't silently miss the modification.
	return "image content changed", nil
}

// formatKeyOrUnknown returns the format key or "unknown" for empty keys.
func formatKeyOrUnknown(f string) string {
	if f == "" {
		return "unknown"
	}
	return f
}
