package binary

import "bytes"

// Preview returns a placeholder indicating binary content.
func (e *BinaryEngine) Preview(data []byte, maxLines int) string {
	return "[binary file]"
}

// Diff compares two binary files by hash.
func (e *BinaryEngine) Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error) {
	if !bytes.Equal(oldData, newData) {
		return "binary files differ", nil
	}
	return "", nil
}
