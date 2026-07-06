package binary

import (
	"bytes"
	"io"
)

// Diff compares two binary files by content. It returns a placeholder
// "binary files differ" message when the bytes are not equal, and an empty
// string when they are identical. Binary files have no line-based diff.
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
