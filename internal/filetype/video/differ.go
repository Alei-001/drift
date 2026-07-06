package video

import (
	"context"
	"fmt"
	"io"

	"github.com/your-org/drift/internal/util/format"
)

// Diff compares two video files by size only, streaming both readers to count
// bytes rather than buffering either file in memory. Video container internals
// are not parsed to avoid pulling in heavyweight decoding dependencies.
// Identical sizes short-circuit to an empty diff.
//
// drift only invokes video Diff on explicitly diffed files, never on bulk
// snapshot diffs. The readers are consumed entirely because the byte count is
// the comparison signal.
func (e *VideoEngine) Diff(ctx context.Context, oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldSize, err := io.Copy(io.Discard, oldReader)
	if err != nil {
		return "", fmt.Errorf("read old video %s: %w", oldPath, err)
	}
	newSize, err := io.Copy(io.Discard, newReader)
	if err != nil {
		return "", fmt.Errorf("read new video %s: %w", newPath, err)
	}
	if oldSize == newSize {
		return "", nil
	}
	return fmt.Sprintf("video file size changed: %s -> %s",
		format.Bytes(oldSize), format.Bytes(newSize)), nil
}
