package core

import (
	"os"
	"path/filepath"
)

func ComputeStatus(commitTree *Tree, idx *Index, workDir string, store StoreReader) (Status, error) {
	s := make(Status)

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

	hasStagedChanges := len(idx.Entries) > 0

	if hasStagedChanges {
		for path, hash := range commitFiles {
			entry, err := idx.Entry(path)
			if err != nil {
				fs := s.File(path)
				fs.Staging = Deleted

				fullPath := filepath.Join(workDir, filepath.FromSlash(path))
				info, statErr := os.Lstat(fullPath)
				if statErr == nil {
					fileHash, hashErr := CalculateHashFromFile(fullPath)
					if hashErr == nil && fileHash != hash {
						fs.Worktree = Modified
					}
					_ = info
				}
				continue
			}
			if entry.Hash != hash {
				fs := s.File(path)
				fs.Staging = Modified
			}
		}

		for _, entry := range idx.Entries {
			hash, inCommit := commitFiles[entry.Path]
			if !inCommit {
				fs := s.File(entry.Path)
				fs.Staging = Added
			} else if hash != entry.Hash {
				fs := s.File(entry.Path)
				fs.Staging = Modified
			}
		}
	}

	if hasStagedChanges {
		for _, entry := range idx.Entries {
			fullPath := filepath.Join(workDir, filepath.FromSlash(entry.Path))
			info, err := os.Lstat(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					fs := s.File(entry.Path)
					fs.Worktree = Deleted
				}
				continue
			}

			if info.ModTime().Equal(entry.ModifiedAt) && info.Size() == entry.Size {
				if s[entry.Path] == nil {
					s.File(entry.Path)
				}
				continue
			}

			hash, err := CalculateHashFromFile(fullPath)
			if err != nil {
				continue
			}

			if hash != entry.Hash {
				fs := s.File(entry.Path)
				fs.Worktree = Modified
			}
		}
	} else {
		for path, hash := range commitFiles {
			fullPath := filepath.Join(workDir, filepath.FromSlash(path))
			_, err := os.Lstat(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					fs := s.File(path)
					fs.Worktree = Deleted
				}
				continue
			}

			fileHash, err := CalculateHashFromFile(fullPath)
			if err != nil {
				continue
			}

			if fileHash != hash {
				fs := s.File(path)
				fs.Worktree = Modified
			}
		}
	}

	err := WalkWorkingDir(workDir, func(path string, info os.FileInfo) error {
		if idx.Has(path) {
			return nil
		}
		if _, inCommit := commitFiles[path]; inCommit {
			return nil
		}
		fs := s.File(path)
		fs.Staging = Untracked
		fs.Worktree = Untracked
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}
