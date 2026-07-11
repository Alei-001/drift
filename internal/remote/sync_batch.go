package remote

import (
	"context"
	"path"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/storage/backends/filesystem"
)

// listRemoteChunkHashes lists the remote chunks directory for the given
// hash prefixes and returns a set of chunk hashes that already exist on
// the remote. Only prefix directories corresponding to chunkHashes are
// listed, so the number of List calls equals the number of distinct
// two-character prefixes in chunkHashes (at most 256, typically far fewer).
//
// If a prefix directory does not exist on the remote (os.ErrNotExist),
// it is treated as empty — all chunks in that prefix are considered
// missing and will be uploaded. Other List errors are also treated as
// empty for that prefix (conservative: will attempt upload, which either
// succeeds or returns a clear error).
func listRemoteChunkHashes(ctx context.Context, rfs RemoteFS, chunkHashes []core.Hash) map[core.Hash]bool {
	prefixes := make(map[string]bool)
	for _, h := range chunkHashes {
		hex := h.FullString()
		prefixes[hex[:2]] = true
	}

	existing := make(map[core.Hash]bool)
	for prefix := range prefixes {
		if err := ctx.Err(); err != nil {
			break
		}
		dirPath := path.Join(filesystem.ChunksDir, prefix)
		entries, err := rfs.List(dirPath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir {
				continue
			}
			name := path.Base(e.Path)
			parent := path.Base(path.Dir(e.Path))
			hexStr := parent + name
			if len(hexStr) != core.HashSize*2 {
				continue
			}
			h, err := parseHashHex(hexStr)
			if err != nil {
				continue
			}
			existing[h] = true
		}
	}
	return existing
}

// listLocalChunkHashes returns a set of chunk hashes that exist in the
// local store. This is a single ListChunks call instead of N per-chunk
// HasChunk calls, significantly reducing I/O for pull operations with
// many chunks.
func listLocalChunkHashes(ctx context.Context, store storage.Storer) map[core.Hash]bool {
	existing := make(map[core.Hash]bool)
	hashes, err := store.ListChunks(ctx)
	if err != nil {
		return existing
	}
	for _, h := range hashes {
		existing[h] = true
	}
	return existing
}
