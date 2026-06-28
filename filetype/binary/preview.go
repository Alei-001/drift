package binary

import (
	"bytes"

	"github.com/zeebo/blake3"
)

// Preview returns a placeholder indicating binary content.
func (e *BinaryEngine) Preview(data []byte, maxLines int) string {
	return "[binary file]"
}

// Diff compares two binary files by hash.
func (e *BinaryEngine) Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error) {
	oldHash := blake3.Sum256(oldData)
	newHash := blake3.Sum256(newData)
	if !bytes.Equal(oldHash[:], newHash[:]) {
		return "binary files differ", nil
	}
	return "", nil
}
