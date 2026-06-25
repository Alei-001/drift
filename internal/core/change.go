package core

import (
	"os"
	"path/filepath"
)

// ComputeStatus compares the commit tree, staging index, and working tree to
// produce a Status describing the state of every tracked and untracked file.
//
// B3: merged the original 3 independent worktree passes into a single union
// loop over staged + committed paths, reducing redundant stat/hash calls.
func ComputeStatus(commitTree *Tree, idx *Index, workDir string, store StoreReader) (Status, error) {
	s := make(Status)

	// 1. Build the set of committed files (path → hash).
	commitFiles := make(map[string]string)
	if commitTree != nil && store != nil {
		reader := NewTreeReader(store)
		blobs, err := reader.ListBlobs(commitTree, "")
		if err != nil {
			return nil, err
		}
		for _, be := range blobs {
			commitFiles[be.Path] = be.Hash
		}
	}

	// 2. Staging status: compare index entries against committed files.
	//
	//    For each file in the index:
	//      - not in commit → Added
	//      - different hash → Modified
	//    For each file in commit:
	//      - not in index  → Deleted
	//      - (same hash is Unmodified, handled by s.File default)
	//
	//    While iterating committed files for the Deleted check, also check
	//    the worktree for modifications to deleted-from-index files (B3:
	//    folded into the same pass instead of a separate loop).
	if len(idx.Entries) > 0 {
		idxSet := make(map[string]int, len(idx.Entries))
		for i, e := range idx.Entries {
			idxSet[e.Path] = i
		}

		for path, hash := range commitFiles {
			if idxIdx, inIdx := idxSet[path]; inIdx {
				if idx.Entries[idxIdx].Hash != hash {
					s.File(path).Staging = Modified
				}
			} else {
				// Deleted from index (staged deletion). The worktree status
				// is relative to the index (which no longer has the file):
				//   - file absent in worktree → Unmodified (matches index)
				//   - file present in worktree → Untracked (handled by step 4)
				// So we do NOT set Worktree=Deleted here — that would compare
				// worktree vs commit instead of worktree vs index.
				s.File(path).Staging = Deleted
			}
		}

		for _, entry := range idx.Entries {
			if _, inCommit := commitFiles[entry.Path]; !inCommit {
				s.File(entry.Path).Staging = Added
			}
		}
	}

	// 3. Worktree status: union of staged + committed paths.
	//
	//    B3: single loop instead of separate staged/committed passes. For
	//    staged paths we can use mtime+size fast-path; for committed-only
	//    paths we fall back to size comparison (no stored mtime).
	{
		seen := make(map[string]bool, len(idx.Entries)+len(commitFiles))

		// Check staged entries.
		for _, entry := range idx.Entries {
			seen[entry.Path] = true
			fullPath := filepath.Join(workDir, filepath.FromSlash(entry.Path))
			info, err := os.Lstat(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					s.File(entry.Path).Worktree = Deleted
				}
				continue
			}

			// mtime+size fast-path (only available for staged files).
			if info.ModTime().Equal(entry.ModifiedAt) && info.Size() == entry.Size {
				continue
			}

			hash, err := CalculateHashFromFile(fullPath)
			if err != nil {
				continue
			}

			if hash != entry.Hash {
				s.File(entry.Path).Worktree = Modified
			}
		}

		// Check committed-but-not-staged files.
		// This only runs when the index is empty (e.g. after `drift unstage`).
		// When the index is non-empty, files in the commit but not in the index
		// are staged for deletion — their worktree status is relative to the
		// index (absent), so Unmodified if also absent from worktree, or
		// Untracked if still present (handled by step 4).
		if len(idx.Entries) == 0 {
			for path, hash := range commitFiles {
				if seen[path] {
					continue
				}
				seen[path] = true

				fullPath := filepath.Join(workDir, filepath.FromSlash(path))
				info, err := os.Lstat(fullPath)
				if err != nil {
					if os.IsNotExist(err) {
						s.File(path).Worktree = Deleted
					}
					continue
				}

				// Symlink: compare target string.
				if info.Mode()&os.ModeSymlink != 0 {
					target, err := os.Readlink(fullPath)
					if err != nil {
						continue
					}
					data, err := store.GetBlob(hash)
					if err != nil {
						continue
					}
					if target != string(data) {
						s.File(path).Worktree = Modified
					}
					continue
				}

				// Size fast-path (no mtime for committed files).
				blobSize, err := store.GetBlobSize(hash)
				if err != nil {
					continue
				}
				if info.Size() != blobSize {
					s.File(path).Worktree = Modified
					continue
				}

				fileHash, err := CalculateHashFromFile(fullPath)
				if err != nil {
					continue
				}

				if fileHash != hash {
					s.File(path).Worktree = Modified
				}
			}
		}
	}

	// 4. Walk working dir for untracked files.
	err := WalkWorkingDir(workDir, func(path string, info os.FileInfo) error {
		if idx.Has(path) {
			return nil
		}
		_, inCommit := commitFiles[path]
		// When the index is empty, committed files are handled by step 3
		// (compared to commit). Don't mark them as Untracked.
		if inCommit && len(idx.Entries) == 0 {
			return nil
		}
		fs := s.File(path)
		if !inCommit {
			// Truly untracked file (not in commit, not in index).
			fs.Staging = Untracked
		}
		// If inCommit and index is non-empty, Staging is already Deleted
		// (staged deletion). The file still in worktree is Untracked.
		fs.Worktree = Untracked
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}
