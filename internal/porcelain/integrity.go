package porcelain

import (
	"context"
	"fmt"

	"github.com/zeebo/blake3"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/glob"
)

// ChunkStatus describes the verification result of a single chunk.
type ChunkStatus string

const (
	ChunkOK      ChunkStatus = "OK"
	ChunkCorrupt ChunkStatus = "CORRUPT (hash mismatch)"
	ChunkMissing ChunkStatus = "MISSING"
)

// ChunkRef records the (snapshot, file, index) context of a chunk reference,
// for verbose output that identifies where each chunk lives.
type ChunkRef struct {
	SnapID   string
	FilePath string
	Idx      int
	Hash     core.Hash
	Status   ChunkStatus
}

// IntegrityReport contains the results of a full repository integrity check.
type IntegrityReport struct {
	TotalBlocks      int
	Corrupt          int
	Missing          int
	SnapshotCorrupt  int
	FileHashMismatch int
	VerboseRefs      []ChunkRef // populated only when verbose=true
}

// VerifyIntegrity verifies the integrity of all chunks in the repository by
// recomputing their BLAKE3 hashes. It collects unique chunk hashes from all
// snapshots, verifies each once, and returns a structured report.
// When filter is non-empty, only files matching the glob pattern are checked.
// When verbose is true, per-chunk references are collected for detailed output.
//
// workDir is accepted for API consistency with other porcelain functions that
// operate on a workspace. The current verification logic is repository-wide and
// does not constrain checks to workDir, but the parameter is retained so future
// per-workspace scoping can be added without breaking callers.
func VerifyIntegrity(ctx context.Context, store storage.Storer, workDir, filter string, verbose bool) (*IntegrityReport, error) {
	_ = workDir // reserved for future per-workspace verification scoping

	var matcher *glob.Matcher
	if filter != "" {
		m, err := glob.Compile(filter)
		if err != nil {
			return nil, fmt.Errorf("invalid filter pattern %q: %w", filter, err)
		}
		matcher = m
	}

	snapshots, err := store.ListSnapshots(ctx, &storage.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	// Collect unique chunk hashes and (for verbose) per-reference context.
	// GetSnapshot also verifies the snapshot's own integrity hash, so a
	// failure here counts towards SnapshotCorrupt.
	hashSet := make(map[core.Hash]bool)
	var refs []ChunkRef
	snapshotCorrupt := 0
	fileHashMismatch := 0
	for _, snap := range snapshots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		full, err := store.GetSnapshot(ctx, snap.ID)
		if err != nil {
			snapshotCorrupt++
			continue
		}
		for _, entry := range full.Files {
			if matcher != nil && !matcher.Match(entry.Path) {
				continue
			}
			// Verify the file-level hash: the entry.Hash must equal
			// computeFileHashFromHashes(entry.Chunks). A mismatch means
			// the recorded hash does not reflect the actual chunk list,
			// which indicates corruption (either the hash or the chunk
			// list was tampered with or written inconsistently).
			if !entry.Hash.IsZero() && entry.Hash != computeFileHashFromHashes(entry.Chunks) {
				fileHashMismatch++
			}
			for idx, hash := range entry.Chunks {
				if hash.IsZero() {
					continue
				}
				hashSet[hash] = true
				if verbose {
					refs = append(refs, ChunkRef{
						SnapID:   full.ShortID(),
						FilePath: entry.Path,
						Idx:      idx,
						Hash:     hash,
					})
				}
			}
		}
	}

	// Verify each unique chunk once; cache the result for verbose output.
	results := make(map[core.Hash]ChunkStatus, len(hashSet))
	report := &IntegrityReport{
		TotalBlocks:      len(hashSet),
		SnapshotCorrupt:  snapshotCorrupt,
		FileHashMismatch: fileHashMismatch,
	}
	for hash := range hashSet {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		has, err := store.HasChunk(ctx, hash)
		if err != nil {
			report.Corrupt++
			results[hash] = ChunkCorrupt
			continue
		}
		if !has {
			report.Missing++
			results[hash] = ChunkMissing
			continue
		}
		chunk, err := store.GetChunk(ctx, hash)
		if err != nil {
			report.Corrupt++
			results[hash] = ChunkCorrupt
			continue
		}
		if core.Hash(blake3.Sum256(chunk.Data)) != hash {
			report.Corrupt++
			results[hash] = ChunkCorrupt
		} else {
			results[hash] = ChunkOK
		}
	}

	if verbose {
		for _, r := range refs {
			r.Status = results[r.Hash]
			report.VerboseRefs = append(report.VerboseRefs, r)
		}
	}

	return report, nil
}
