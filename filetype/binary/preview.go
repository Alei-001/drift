package binary

import (
	"bytes"
	"io"
)

// Preview returns a placeholder indicating binary content.
func (e *BinaryEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
	_ = header
	_ = size
	_ = reader
	_ = maxLines
	return "[binary file]", nil
}

// Diff compares two binary files by hash.
func (e *BinaryEngine) Diff(oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldData, err := io.ReadAll(oldReader)
	if err != nil {
		return "", err
	}
	newData, err := io.ReadAll(newReader)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(oldData, newData) {
		return "binary files differ", nil
	}
	return "", nil
}
