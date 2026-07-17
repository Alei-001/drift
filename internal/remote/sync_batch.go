package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
)

// listRemoteChunkHashes lists the remote chunks directory for the given
// hash prefixes and returns a set of chunk hashes that already exist on
// the remote. Only prefix directories corresponding to chunkHashes are
// listed, so the number of List calls equals the number of distinct
// two-character prefixes in chunkHashes (at most 256, typically far fewer).
//
// If a prefix directory does not exist on the remote (os.ErrNotExist),
// it is treated as empty — all chunks in that prefix are considered
// missing and will be uploaded. Other List errors (network, auth, etc.)
// are returned immediately so the caller can surface them rather than
// silently proceeding with an incomplete existence set.
func listRemoteChunkHashes(ctx context.Context, rfs RemoteFS, chunkHashes []core.Hash) (map[core.Hash]bool, error) {
	prefixes := make(map[string]bool)
	for _, h := range chunkHashes {
		hex := h.FullString()
		prefixes[hex[:2]] = true
	}

	existing := make(map[core.Hash]bool)
	for prefix := range prefixes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		dirPath := path.Join(store.ChunksDir, prefix)
		entries, err := rfs.List(ctx, dirPath)
		if err != nil {
			// Directory not existing on the remote is normal — it means
			// no chunks with that prefix have been uploaded yet.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			// Other errors (network, auth, permission) must not be
			// silently swallowed: they indicate a real problem that
			// would cause the sync to proceed with incorrect state.
			return nil, fmt.Errorf("list remote chunks/%s: %w", prefix, err)
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
	return existing, nil
}

// listLocalChunkHashes returns a set of chunk hashes that exist in the
// local store. This is a single ListChunks call instead of N per-chunk
// HasChunk calls, significantly reducing I/O for pull operations with
// many chunks. A ListChunks failure is returned rather than silently
// producing an empty set, which would cause pull to re-download every
// chunk and could mask a real storage problem.
func listLocalChunkHashes(ctx context.Context, st *store.StoreSet) (map[core.Hash]bool, error) {
	existing := make(map[core.Hash]bool)
	hashes, err := st.Chunks.ListChunks(ctx)
	if err != nil {
		return nil, fmt.Errorf("list local chunks: %w", err)
	}
	for _, h := range hashes {
		existing[h] = true
	}
	return existing, nil
}
