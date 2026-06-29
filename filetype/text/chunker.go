package text

import (
	"github.com/your-org/drift/chunker"
)

// textWholeFileThreshold: text files smaller than this are stored as a single
// whole-file chunk. Small text files benefit from avoiding FastCDC overhead.
const textWholeFileThreshold = 64 * 1024 // 64KB

// ChunkerFor returns the chunking strategy for a text file of the given size.
// Files below textWholeFileThreshold return nil (whole-file single chunk);
// larger files use fine-grained FastCDC (4K/8K/16K) to maximize dedup of
// small shared regions in source files.
func (e *TextEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	if fileSize < textWholeFileThreshold {
		return nil
	}
	return chunker.NewFastCDCChunkerWithParams(4096, 8192, 16384)
}
