package binary

import "io"

// Preview returns a placeholder indicating binary content. Binary files have
// no meaningful text preview.
func (e *BinaryEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
	_ = header
	_ = size
	_ = reader
	_ = maxLines
	return "[binary file]", nil
}
