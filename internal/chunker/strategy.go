package chunker

// DefaultSelector is the default Chunker selector: it delegates to
// BinaryChunkerFor for all files.
type DefaultSelector struct{}

// ChunkerFor returns a Chunker appropriate for the given file size. It
// currently always delegates to BinaryChunkerFor.
func (DefaultSelector) ChunkerFor(fileSize int64) Chunker {
	return BinaryChunkerFor(fileSize)
}

// binaryLargeThreshold and binaryHugeThreshold are the file-size cutoffs that
// select which chunking strategy to use. They are unexported because no
// external package needs to reference them — BinaryChunkerFor encapsulates
// the entire size-based dispatch.
const (
	binaryLargeThreshold = 50 * 1024 * 1024   // 50MB
	binaryHugeThreshold  = 500 * 1024 * 1024  // 500MB
)

// BinaryChunkerFor returns a Chunker tuned for binary file content of the
// given size. Files below binaryLargeThreshold use FastCDC with the package
// defaults (128/256/512 KB via fastCDCDefault*); files between
// binaryLargeThreshold and binaryHugeThreshold use FastCDC with sizes scaled
// 4×; files at or above binaryHugeThreshold use fixed chunking derived from
// the default average size scaled 8×.
func BinaryChunkerFor(fileSize int64) Chunker {
	minSize, avgSize, maxSize := fastCDCDefaultMinSize, fastCDCDefaultAvgSize, fastCDCDefaultMaxSize

	switch {
	case fileSize < binaryLargeThreshold:
		return NewFastCDCChunkerWithParams(minSize, avgSize, maxSize)
	case fileSize < binaryHugeThreshold:
		// Scale all three proportionally to keep the min/avg/max ratio
		// while producing fewer, larger chunks for big files.
		return NewFastCDCChunkerWithParams(minSize*4, avgSize*4, maxSize*4)
	default:
		// For huge files, use fixed chunking with a size derived from
		// the default avg, scaled up to reduce chunk count.
		return NewFixedChunker(avgSize * 8)
	}
}
