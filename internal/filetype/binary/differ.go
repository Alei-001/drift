package binary

import (
	"bytes"
	"context"
	"io"
)

// binaryDiffBufSize is the streaming comparison buffer. Each pass reads at
// most this many bytes from both readers, so memory stays constant regardless
// of file size.
const binaryDiffBufSize = 32 * 1024

// Diff compares two binary files by content streaming from oldReader/newReader
// rather than buffering either file in memory. It returns a placeholder
// "binary files differ" message when the bytes are not equal, and an empty
// string when they are identical. Binary files have no line-based diff.
func (e *BinaryEngine) Diff(ctx context.Context, oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldBuf := make([]byte, binaryDiffBufSize)
	newBuf := make([]byte, binaryDiffBufSize)
	for {
		oldN, oldErr := io.ReadFull(oldReader, oldBuf)
		newN, newErr := io.ReadFull(newReader, newBuf)

		// Propagate any real (non-EOF) reader error. io.ReadFull never
		// wraps these, so direct comparison with the io sentinels is safe
		// (matching the pattern in internal/storage/stream/stream.go).
		if oldErr != nil && oldErr != io.EOF && oldErr != io.ErrUnexpectedEOF {
			return "", oldErr
		}
		if newErr != nil && newErr != io.EOF && newErr != io.ErrUnexpectedEOF {
			return "", newErr
		}
		// A broken underlying reader surfaces as (0, io.ErrUnexpectedEOF),
		// which is distinct from a legitimate partial final read
		// (n>0, io.ErrUnexpectedEOF) produced by io.ReadFull when the stream
		// ends before filling the buffer. Propagate the broken-reader case.
		if oldN == 0 && oldErr == io.ErrUnexpectedEOF {
			return "", oldErr
		}
		if newN == 0 && newErr == io.ErrUnexpectedEOF {
			return "", newErr
		}

		if oldN != newN || !bytes.Equal(oldBuf[:oldN], newBuf[:newN]) {
			return "binary files differ", nil
		}

		oldDone := oldErr == io.EOF || oldErr == io.ErrUnexpectedEOF
		newDone := newErr == io.EOF || newErr == io.ErrUnexpectedEOF
		if oldDone && newDone {
			return "", nil
		}
		if oldDone != newDone {
			return "binary files differ", nil
		}
	}
}
