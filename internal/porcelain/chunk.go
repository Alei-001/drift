package porcelain

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/filetype"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/zeebo/blake3"
)

// wholeFileChunkThreshold is the maximum file size that chunkFile will read
// as a single chunk on the nil-chunker path. It matches TextEngine's
// whole-file threshold: files larger than this require a real chunker to
// avoid buffering the entire file in memory.
const wholeFileChunkThreshold = 64 * 1024

// chunkFile chunks a file using the engine-selected chunker, invoking fn
// for each chunk as it is produced. If the engine returns a nil chunker (or
// the file is empty), the whole file is read as a single chunk. Large files
// (>64 KB) are rejected on the nil-chunker path to avoid OOM; engines that
// return nil are expected to do so only for small files (see
// TextEngine.ChunkerFor). If fn returns an error, chunkFile stops and
// returns it.
//
// Nil-chunker protection: the explicit `if c == nil` branch below is the
// guard that prevents a nil-dereference panic when engine.ChunkerFor
// returns nil. Callers also check engine == nil before invoking chunkFile
// (see processFileTask and ComputeFileHash), but this function adds a
// defense-in-depth check so a future refactor that bypasses those callers
// cannot reach the c.Chunk call with a nil receiver.
func chunkFile(ctx context.Context, path string, r io.Reader, engine filetype.Engine, fileSize int64, fn func(*core.Chunk) error) error {
	if engine == nil {
		// Defense-in-depth: callers should have caught this already,
		// but a nil engine here would panic on ChunkerFor below.
		return fmt.Errorf("no chunker available for %s", path)
	}
	c := engine.ChunkerFor(fileSize)
	if fileSize == 0 {
		c = nil
	}
	if c == nil {
		// Reject large files before reading them into memory. The
		// nil-chunker path reads the whole file as a single chunk, so
		// a 500 MB video would OOM. 64 KB matches TextEngine's
		// whole-file threshold.
		if fileSize > wholeFileChunkThreshold {
			return fmt.Errorf("file %s too large (%d bytes) for whole-file chunking without chunker", path, fileSize)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		// core.Chunk.Size is uint32; reject files whose single-chunk
		// representation would overflow it. In practice this path is
		// only reached for small text files (< 64KB), but guard
		// defensively in case a future engine returns nil for a
		// large file.
		if uint64(len(data)) > math.MaxUint32 {
			return fmt.Errorf("file too large for single-chunk storage (%d bytes)", len(data))
		}
		sum := blake3.Sum256(data)
		var hash core.Hash
		copy(hash[:], sum[:])
		chunk := &core.Chunk{
			Hash:  hash,
			Size:  uint32(len(data)),
			Data:  data,
			Flags: core.ChunkFlagNone,
		}
		return fn(chunk)
	}
	return c.Chunk(ctx, r, fn)
}

// CountFileLines returns the total number of lines in the file represented by
// entry, by counting newline bytes across all of its chunks.
//
// Error handling: a missing chunk or a cancelled context causes the function
// to return 0. This silent fallback is intentional for the current callers
// (log -v / log --json), which treat line counting as best-effort decoration
// and prefer a missing count over aborting the whole log listing. Callers
// that need explicit error handling should use CountFileLinesWithError.
func CountFileLines(ctx context.Context, store storage.Storer, entry core.FileEntry) int {
	count, _ := CountFileLinesWithError(ctx, store, entry)
	return count
}

// CountFileLinesWithError is like CountFileLines but returns an error when a
// chunk cannot be read or the context is cancelled, so callers that need to
// distinguish "zero lines" from "failed to count" can do so.
func CountFileLinesWithError(ctx context.Context, store storage.Storer, entry core.FileEntry) (int, error) {
	count := 0
	for _, h := range entry.Chunks {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		chunk, err := store.GetChunk(ctx, h)
		if err != nil {
			return 0, err
		}
		count += bytes.Count(chunk.Data, []byte{'\n'})
	}
	return count, nil
}

// computeFileHashFromHashes derives the file-level hash by hashing the
// concatenation of chunk hashes. This makes the file hash independent of
// chunk storage format (compression, flags), not chunk boundaries —
// the same file chunked with different cut points produces different
// chunk hashes and thus a different file hash. CreateSnapshot and
// ComputeFileHash produce identical hashes for the same file because
// they use the same chunker with the same parameters.
func computeFileHashFromHashes(hashes []core.Hash) core.Hash {
	fileHasher := blake3.New()
	for _, h := range hashes {
		fileHasher.Write(h[:])
	}
	var fileHash core.Hash
	copy(fileHash[:], fileHasher.Sum(nil))
	return fileHash
}
