package image

import (
	"bytes"
	"fmt"
	"io"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/util/format"
	"github.com/zeebo/blake3"
)

// Diff compares two images streaming, without buffering either file wholly in
// memory. The leading core.HeaderPeekSize bytes are peeked for format and
// dimension detection; the remainder is hashed with BLAKE3 for an equality
// short-circuit. Priority of reported changes is:
//
//	format -> dimensions -> size -> content
//
// Returns an empty string when the images are identical (same size and hash).
//
// The full file bytes are never held in memory: only the fixed-size header
// buffer and the 32-byte hash digest are retained, so memory stays constant
// regardless of image size.
func (e *ImageEngine) Diff(oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldHeader, oldRest, err := peekDiffHeader(oldReader)
	if err != nil {
		return "", fmt.Errorf("read old image %s: %w", oldPath, err)
	}
	newHeader, newRest, err := peekDiffHeader(newReader)
	if err != nil {
		return "", fmt.Errorf("read new image %s: %w", newPath, err)
	}

	oldSize, oldHash, err := hashAndSize(oldRest)
	if err != nil {
		return "", fmt.Errorf("read old image %s: %w", oldPath, err)
	}
	newSize, newHash, err := hashAndSize(newRest)
	if err != nil {
		return "", fmt.Errorf("read new image %s: %w", newPath, err)
	}

	// Identity short-circuit: same size + same BLAKE3 hash means identical
	// content. This replaces the prior bytes.Equal whole-file check.
	if oldSize == newSize && bytes.Equal(oldHash, newHash) {
		return "", nil
	}

	oldFormat := detectFormatByMagic(oldHeader)
	newFormat := detectFormatByMagic(newHeader)
	if oldFormat != newFormat {
		return fmt.Sprintf("image format changed: %s -> %s",
			formatKeyOrUnknown(oldFormat), formatKeyOrUnknown(newFormat)), nil
	}

	oldW, oldH := decodeDimensions(oldHeader)
	newW, newH := decodeDimensions(newHeader)
	if oldW != newW || oldH != newH {
		return fmt.Sprintf("image dimensions changed: %dx%d -> %dx%d",
			oldW, oldH, newW, newH), nil
	}

	if oldSize != newSize {
		return fmt.Sprintf("image file size changed: %s -> %s",
			format.Bytes(oldSize), format.Bytes(newSize)), nil
	}

	// Same format, dimensions, and size, but the hash differs — the bytes
	// genuinely changed without altering any structural property.
	return "image content changed", nil
}

// peekDiffHeader reads up to core.HeaderPeekSize bytes from r and returns the
// header along with a reader that replays the header followed by the unread
// remainder of r. This lets the caller inspect magic bytes and dimensions and
// then continue hashing the full stream without re-reading the file.
func peekDiffHeader(r io.Reader) (header []byte, rest io.Reader, err error) {
	buf := make([]byte, core.HeaderPeekSize)
	got, err := io.ReadFull(r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		// io.ReadFull signals a short final read with io.ErrUnexpectedEOF
		// and an empty stream with io.EOF; both are normal for a header
		// peek of a small file. A genuinely broken reader that returns
		// io.ErrUnexpectedEOF with zero bytes is detected below by the
		// subsequent io.Copy in hashAndSize, which will surface the error.
		err = nil
	} else if err != nil {
		return nil, nil, err
	}
	header = buf[:got]
	return header, io.MultiReader(bytes.NewReader(header), r), nil
}

// hashAndSize streams r through a BLAKE3 hasher while counting bytes, returning
// the total size and the resulting 32-byte digest. Neither the file content
// nor the hasher state is retained after the call, keeping memory constant.
func hashAndSize(r io.Reader) (int64, []byte, error) {
	h := blake3.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return 0, nil, err
	}
	return n, h.Sum(nil), nil
}

// formatKeyOrUnknown returns the format key or "unknown" for empty keys.
func formatKeyOrUnknown(f string) string {
	if f == "" {
		return "unknown"
	}
	return f
}
