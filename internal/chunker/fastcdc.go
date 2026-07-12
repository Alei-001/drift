package chunker

import (
	"context"
	"io"

	"github.com/Alei-001/drift/internal/core"
	cdc "github.com/PlakarKorp/go-cdc-chunkers"
	_ "github.com/PlakarKorp/go-cdc-chunkers/chunkers/fastcdc"
	"github.com/zeebo/blake3"
)

// fastCDCDefaultMinSize etc. alias the core-level defaults so this package
// has a single source of truth for chunk sizes (see core.DefaultChunk*Size).
const (
	fastCDCDefaultMinSize = core.DefaultChunkMinSize
	fastCDCDefaultAvgSize = core.DefaultChunkAvgSize
	fastCDCDefaultMaxSize = core.DefaultChunkMaxSize
	// fastCDCMinSizeFloor is the minimum value accepted by the underlying
	// FastCDC library (it rejects MinSize < 64). We clamp to this floor so
	// callers can never trip a library validation error.
	fastCDCMinSizeFloor = 64
)

// FastCDCChunker implements Chunker using the FastCDC content-defined
// chunking algorithm. Cut points depend on the data itself, so identical
// regions of two files produce the same chunks.
type FastCDCChunker struct {
	minSize int
	avgSize int
	maxSize int
}

// NewFastCDCChunker creates a FastCDC chunker with the default
// 128KB/256KB/512KB min/avg/max sizes. Behavior is unchanged from before
// this constructor accepted parameters.
func NewFastCDCChunker() *FastCDCChunker {
	return NewFastCDCChunkerWithParams(fastCDCDefaultMinSize, fastCDCDefaultAvgSize, fastCDCDefaultMaxSize)
}

// NewFastCDCChunkerWithParams creates a FastCDC chunker with custom
// min/avg/max chunk sizes. Parameters are clamped to satisfy the underlying
// FastCDC library constraints (MinSize >= 64B, NormalSize must be a power of
// two, MinSize < NormalSize < MaxSize), so any input produces a usable
// chunker without panicking.
func NewFastCDCChunkerWithParams(minSize, avgSize, maxSize int) *FastCDCChunker {
	minSize, avgSize, maxSize = clampFastCDCParams(minSize, avgSize, maxSize)
	return &FastCDCChunker{
		minSize: minSize,
		avgSize: avgSize,
		maxSize: maxSize,
	}
}

// clampFastCDCParams normalizes min/avg/max sizes against both the task's
// clamp rules and the FastCDC library constraints.
func clampFastCDCParams(minSize, avgSize, maxSize int) (int, int, int) {
	// minSize: negative -> default minimum (512B); enforce library floor of 64B.
	if minSize < 0 {
		minSize = 512
	}
	if minSize < fastCDCMinSizeFloor {
		minSize = fastCDCMinSizeFloor
	}

	// maxSize: non-positive or smaller than minSize -> default 512KB.
	if maxSize <= 0 || maxSize < minSize {
		maxSize = fastCDCDefaultMaxSize
	}

	// If minSize and maxSize are still inconsistent (minSize >= maxSize,
	// e.g. the user passed a minSize larger than the default max), fall
	// back to the default pair so a valid range always exists.
	if minSize >= maxSize {
		minSize = fastCDCDefaultMinSize
		maxSize = fastCDCDefaultMaxSize
	}

	// avgSize: must be a power of two strictly between minSize and maxSize.
	// The task specifies (minSize+maxSize)/2 as the fallback target, but the
	// library requires a power of two, so we round to the nearest power of
	// two that fits in (minSize, maxSize).
	if !isPowerOfTwo(avgSize) || avgSize <= minSize || avgSize >= maxSize {
		avgSize = nearestPowerOfTwoBetween(minSize, maxSize)
		if avgSize <= minSize || avgSize >= maxSize {
			// No valid power of two exists between minSize and maxSize;
			// fall back to the full default triple.
			minSize = fastCDCDefaultMinSize
			avgSize = fastCDCDefaultAvgSize
			maxSize = fastCDCDefaultMaxSize
		}
	}

	return minSize, avgSize, maxSize
}

func isPowerOfTwo(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

// nearestPowerOfTwoBetween returns the power of two closest to
// (minVal+maxVal)/2 that lies strictly between minVal and maxVal.
// It returns 0 if no such power of two exists.
func nearestPowerOfTwoBetween(minVal, maxVal int) int {
	target := (minVal + maxVal) / 2
	lower := 1
	for lower*2 <= target {
		lower *= 2
	}
	upper := lower * 2

	var best int
	if target-lower <= upper-target {
		best = lower
	} else {
		best = upper
	}
	if best > minVal && best < maxVal {
		return best
	}
	if upper > minVal && upper < maxVal {
		return upper
	}
	if lower > minVal && lower < maxVal {
		return lower
	}
	for p := lower / 2; p > minVal; p /= 2 {
		if p > minVal && p < maxVal {
			return p
		}
	}
	return 0
}

// Chunk splits r into content-defined chunks using FastCDC. Each chunk is
// BLAKE3-hashed and zero-length chunks are skipped. The returned slice is
// empty for an empty reader. The context is checked at each cut point so a
// cancelled context aborts the chunking loop promptly.
func (f *FastCDCChunker) Chunk(ctx context.Context, r io.Reader) ([]*core.Chunk, error) {
	// Use "fastcdc-v1.0.0" instead of legacy "fastcdc": the legacy mode
	// forces hardcoded masks computed for an 8KB NormalSize, which would
	// skew cut points for our 128KB/256KB/512KB sizes. The v1.0.0 variant
	// computes masks dynamically from the actual NormalSize.
	ch, err := cdc.NewChunker("fastcdc-v1.0.0", r, &cdc.ChunkerOpts{
		MinSize:    f.minSize,
		NormalSize: f.avgSize,
		MaxSize:    f.maxSize,
	})
	if err != nil {
		return nil, err
	}

	var chunks []*core.Chunk

	err = ch.Split(func(offset, length uint, chunkData []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// The chunker may emit a zero-length chunk (e.g. for an empty
		// stream); skip it so callers never see empty chunks.
		if length == 0 {
			return nil
		}
		var hash core.Hash
		sum := blake3.Sum256(chunkData)
		copy(hash[:], sum[:])

		chunk := &core.Chunk{
			Hash:  hash,
			Size:  uint32(length),
			Data:  make([]byte, length),
			Flags: core.ChunkFlagNone,
		}
		copy(chunk.Data, chunkData)

		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return chunks, nil
}
