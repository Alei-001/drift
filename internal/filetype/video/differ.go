package video

import (
	"bytes"
	"fmt"
	"io"

	"github.com/your-org/drift/internal/util/format"
)

// Diff compares two video files by size only. Video container internals are
// not parsed to avoid pulling in heavyweight decoding dependencies. Identical
// bytes (checked via bytes.Equal) short-circuit to an empty diff.
//
// Both readers are consumed entirely because equality and length both
// require the full byte count. drift only invokes video Diff on explicitly
// diffed files, never on bulk snapshot diffs.
func (e *VideoEngine) Diff(oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldData, err := io.ReadAll(oldReader)
	if err != nil {
		return "", fmt.Errorf("read old video %s: %w", oldPath, err)
	}
	newData, err := io.ReadAll(newReader)
	if err != nil {
		return "", fmt.Errorf("read new video %s: %w", newPath, err)
	}
	if bytes.Equal(oldData, newData) {
		return "", nil
	}
	oldSize := format.Bytes(int64(len(oldData)))
	newSize := format.Bytes(int64(len(newData)))
	return fmt.Sprintf("video file size changed: %s -> %s", oldSize, newSize), nil
}

