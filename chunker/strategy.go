package chunker

// Size thresholds for the binary-class chunking strategy shared by
// image/video/binary engines.
const (
	// BinaryLargeThreshold: files at or above this size use larger FastCDC
	// targets (1M/2M/4M) to keep the chunk count manageable.
	BinaryLargeThreshold = 50 * 1024 * 1024 // 50MB
	// BinaryHugeThreshold: files at or above this size switch to fixed 8MB
	// chunks for predictable, bounded memory and chunk count.
	BinaryHugeThreshold = 500 * 1024 * 1024 // 500MB
)

// BinaryChunkerFor returns a chunker for binary-class files
// (image/video/binary) using a 3-tier strategy based on file size:
//   - < 50MB:   default FastCDC (128K/256K/512K) balances dedup granularity
//     and chunk count for typical media/binaries.
//   - 50-500MB: larger FastCDC (1M/2M/4M) keeps the chunk count reasonable as
//     file sizes grow.
//   - >= 500MB: fixed 8MB chunks give predictable, bounded memory and chunk
//     count for very large files.
func BinaryChunkerFor(fileSize int64) Chunker {
	switch {
	case fileSize < BinaryLargeThreshold:
		return NewFastCDCChunker()
	case fileSize < BinaryHugeThreshold:
		return NewFastCDCChunkerWithParams(1 << 20, 2 << 20, 4 << 20)
	default:
		return NewFixedChunker(8 << 20)
	}
}
