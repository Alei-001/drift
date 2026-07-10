package chunker

// ── Strategy overview ────────────────────────────────────────────────────────
//
// This file provides two public building blocks:
//
//   BinaryChunkerFor(fileSize)
//     The canonical, size-adaptive chunking function for binary/image/video
//     content. Filetype engines call this directly from their ChunkerFor
//     implementation. It is a pure function — stateless, no interface.
//
//   DefaultSelector
//     A convenience adapter that satisfies filetype.ChunkerSelector by
//     delegating to BinaryChunkerFor. Use it in new engine scaffolding so the
//     engine compiles before you write a custom ChunkerFor, or for any engine
//     whose chunking needs are identical to the binary default.
//
// Text engines implement their own ChunkerFor (e.g. 4K-8K-16K FastCDC params)
// rather than using DefaultSelector, because source-code-friendly chunk sizes
// differ from binary defaults.
//
// ─────────────────────────────────────────────────────────────────────────────

// binaryLargeThreshold and binaryHugeThreshold are the file-size cutoffs that
// select which chunking strategy to use. They are unexported because no
// external package needs to reference them — BinaryChunkerFor encapsulates
// the entire size-based dispatch.
const (
	binaryLargeThreshold = 50 * 1024 * 1024  // 50 MB
	binaryHugeThreshold  = 500 * 1024 * 1024 // 500 MB
)

// BinaryChunkerFor returns a Chunker tuned for binary file content of the
// given size. It is the canonical building block for image, video, and binary
// filetype engines.
//
//   - size < 50 MB  → FastCDC with default params (128/256/512 KB)
//   - 50 MB ≤ size < 500 MB → FastCDC with params scaled 4× (fewer, larger chunks)
//   - size ≥ 500 MB → FixedChunker at avgSize×8 to cap chunk-count overhead
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

// DefaultSelector is a convenience adapter that satisfies the
// filetype.ChunkerSelector interface by delegating to BinaryChunkerFor.
// Use it in engine scaffolding or for any engine whose chunking requirements
// match the binary defaults. Engines with custom size thresholds (e.g. text)
// should implement ChunkerFor directly instead of using DefaultSelector.
type DefaultSelector struct{}

// ChunkerFor returns a Chunker appropriate for the given file size by
// delegating to BinaryChunkerFor.
func (DefaultSelector) ChunkerFor(fileSize int64) Chunker {
	return BinaryChunkerFor(fileSize)
}
