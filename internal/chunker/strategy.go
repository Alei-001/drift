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
//     A convenience adapter that satisfies engine.ChunkerSelector by
//     delegating to BinaryChunkerFor. Use it in new engine scaffolding so the
//     engine compiles before you write a custom ChunkerFor, or for any engine
//     whose chunking needs are identical to the binary default.
//
// Text engines implement their own ChunkerFor (e.g. 4K-8K-16K FastCDC params)
// rather than using DefaultSelector, because source-code-friendly chunk sizes
// differ from binary defaults.
//
// ─────────────────────────────────────────────────────────────────────────────

// binaryChunkThresholds are the file-size cutoffs that select which FastCDC
// parameters to use. All tiers use content-defined chunking — the deduplication
// guarantee is never lost, regardless of file size.
const (
	binaryMediumThreshold = 50 * 1024 * 1024  // 50 MB
	binaryLargeThreshold  = 200 * 1024 * 1024 // 200 MB
	binaryHugeThreshold   = 1 * 1024 * 1024 * 1024 // 1 GB
)

// BinaryChunkerFor returns a Chunker tuned for binary file content of the
// given size. All tiers use FastCDC content-defined chunking so that the
// core deduplication guarantee is never lost — identical content always
// produces identical chunk hashes, regardless of file size.
//
//   - size < 50 MB  → FastCDC 128K/256K/512K
//   - 50 MB ≤ size < 200 MB → FastCDC 256K/512K/1M
//   - 200 MB ≤ size < 1 GB → FastCDC 512K/1M/2M
//   - size ≥ 1 GB → FastCDC 1M/2M/4M
func BinaryChunkerFor(fileSize int64) Chunker {
	minSize, avgSize, maxSize := fastCDCDefaultMinSize, fastCDCDefaultAvgSize, fastCDCDefaultMaxSize

	switch {
	case fileSize < binaryMediumThreshold:
		return NewFastCDCChunkerWithParams(minSize, avgSize, maxSize)
	case fileSize < binaryLargeThreshold:
		return NewFastCDCChunkerWithParams(minSize*2, avgSize*2, maxSize*2)
	case fileSize < binaryHugeThreshold:
		return NewFastCDCChunkerWithParams(minSize*4, avgSize*4, maxSize*4)
	default:
		return NewFastCDCChunkerWithParams(minSize*8, avgSize*8, maxSize*8)
	}
}

// DefaultSelector is a convenience adapter that satisfies the
// engine.ChunkerSelector interface by delegating to BinaryChunkerFor.
// Use it in engine scaffolding or for any engine whose chunking requirements
// match the binary defaults. Engines with custom size thresholds (e.g. text)
// should implement ChunkerFor directly instead of using DefaultSelector.
type DefaultSelector struct{}

// ChunkerFor returns a Chunker appropriate for the given file size by
// delegating to BinaryChunkerFor.
func (DefaultSelector) ChunkerFor(fileSize int64) Chunker {
	return BinaryChunkerFor(fileSize)
}
