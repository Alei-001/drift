package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
)

type RemoveOptions struct {
	Cached    bool
	Recursive bool
	DryRun    bool
}

type MoveOptions struct {
	Force bool
}

type CleanOptions struct {
	DryRun bool
	Dirs   bool
}

func (a *App) Remove(paths []string, opts RemoveOptions) ([]string, error) {
	expanded, err := a.expandRemovePaths(paths, opts.Recursive)
	if err != nil {
		return nil, err
	}
	if len(expanded) == 0 {
		return nil, fmt.Errorf("no matching files found")
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	tracked := make(map[string]bool)
	for _, e := range idx.Entries {
		tracked[e.Path] = true
	}
	parentHashes, err := a.wt.LoadParentTreeHashes()
	if err != nil {
		return nil, fmt.Errorf("failed to load tracked paths: %w", err)
	}
	for p := range parentHashes {
		tracked[p] = true
	}

	var toRemove []string
	for _, p := range expanded {
		if tracked[p] {
			toRemove = append(toRemove, p)
		}
	}

	if len(toRemove) == 0 {
		return nil, fmt.Errorf("no tracked files matched")
	}

	// DryRun: return list of files that would be removed without actually removing them
	if opts.DryRun {
		return toRemove, nil
	}

	for _, p := range toRemove {
		idx.Remove(p)
		if !opts.Cached {
			fullPath := filepath.Join(a.dir, filepath.FromSlash(p))
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to remove %s: %w", p, err)
			}
		}
	}

	if err := a.store.SaveIndex(&idx); err != nil {
		return nil, fmt.Errorf("failed to save index: %w", err)
	}

	if !opts.Cached {
		a.wt.CleanEmptyDirs(toRemove)
	}

	return toRemove, nil
}

func (a *App) expandRemovePaths(args []string, recursive bool) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	addPath := func(rel string) {
		if !seen[rel] {
			seen[rel] = true
			result = append(result, rel)
		}
	}

	collectDir := func(dirRel string) error {
		fullDir := filepath.Join(a.dir, filepath.FromSlash(dirRel))
		if err := core.WalkWorkingDirWithIgnore(fullDir, a.dir, func(path string, info os.FileInfo) error {
			relPath := filepath.ToSlash(filepath.Join(dirRel, path))
			addPath(relPath)
			return nil
		}); err != nil {
			return fmt.Errorf("failed to walk directory %s: %w", dirRel, err)
		}
		return nil
	}

	for _, arg := range args {
		if strings.ContainsAny(arg, "*?[") {
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
			}
			for _, m := range matches {
				absPath, err := filepath.Abs(m)
				if err != nil {
					absPath = m
				}
				rel, err := filepath.Rel(a.dir, absPath)
				if err != nil {
					rel = m
				}
				rel = filepath.ToSlash(rel)
				info, err := os.Lstat(m)
				if err != nil {
					continue
				}
				if info.IsDir() {
					if !recursive {
						continue
					}
					if err := collectDir(rel); err != nil {
						return nil, err
					}
					continue
				}
				addPath(rel)
			}
			continue
		}

		info, err := os.Lstat(arg)
		if err != nil {
			rel := filepath.ToSlash(arg)
			addPath(rel)
			continue
		}

		absPath, err := filepath.Abs(arg)
		if err != nil {
			absPath = arg
		}
		rel, err := filepath.Rel(a.dir, absPath)
		if err != nil {
			rel = arg
		}
		rel = filepath.ToSlash(rel)

		if info.IsDir() {
			if !recursive {
				return nil, fmt.Errorf("not removing recursively without -r: %s", rel)
			}
			if err := collectDir(rel); err != nil {
				return nil, err
			}
			continue
		}

		addPath(rel)
	}

	return result, nil
}

func (a *App) Move(sources []string, dest string, opts MoveOptions) error {
	destInfo, destErr := os.Lstat(filepath.Join(a.dir, filepath.FromSlash(dest)))
	destIsDir := destErr == nil && destInfo.IsDir()

	if !destIsDir && len(sources) > 1 {
		return fmt.Errorf("moving multiple sources requires the destination to be a directory")
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return fmt.Errorf("failed to load index: %w", err)
	}

	tracked := make(map[string]bool)
	for _, e := range idx.Entries {
		tracked[e.Path] = true
	}
	parentHashes, err := a.wt.LoadParentTreeHashes()
	if err != nil {
		return fmt.Errorf("failed to load parent tree: %w", err)
	}
	for p := range parentHashes {
		tracked[p] = true
	}

	for _, src := range sources {
		// Best-effort: these functions rarely fail for normal paths.
		absSrc, _ := filepath.Abs(src)
		srcRel, _ := filepath.Rel(a.dir, absSrc)
		srcRel = filepath.ToSlash(srcRel)
		if !tracked[srcRel] {
			return fmt.Errorf("source '%s' is not tracked (use 'drift add' first)", srcRel)
		}

		var destRel string
		if destIsDir {
			destRel = filepath.ToSlash(filepath.Join(dest, filepath.Base(srcRel)))
		} else {
			destRel = filepath.ToSlash(dest)
		}

		if err := core.ValidateTreePath(destRel); err != nil {
			return fmt.Errorf("invalid destination %q: %w", destRel, err)
		}

		srcFull := filepath.Join(a.dir, filepath.FromSlash(srcRel))
		destFull := filepath.Join(a.dir, filepath.FromSlash(destRel))

		if err := os.MkdirAll(filepath.Dir(destFull), 0755); err != nil {
			return fmt.Errorf("failed to create destination directory: %w", err)
		}

		if _, err := os.Stat(destFull); err == nil {
			if !opts.Force {
				return fmt.Errorf("destination exists (use -f to overwrite): %s", destRel)
			}
			if err := os.Remove(destFull); err != nil {
				return fmt.Errorf("failed to remove existing destination %s: %w", destRel, err)
			}
		}

		if err := os.Rename(srcFull, destFull); err != nil {
			return fmt.Errorf("failed to move %s to %s: %w", srcRel, destRel, err)
		}

		if entry, err := idx.Entry(srcRel); err == nil {
			newEntry := entry
			newEntry.Path = destRel
			if err := idx.Add(newEntry); err != nil {
				return fmt.Errorf("failed to stage %s: %w", destRel, err)
			}
			idx.Remove(srcRel)
		} else {
			info, err := os.Lstat(destFull)
			if err != nil {
				return fmt.Errorf("failed to stat moved file: %w", err)
			}
			mode, err := core.NormalizeModeForPath(info.Mode(), destRel)
			if err != nil {
				return fmt.Errorf("unsupported file type for %s: %w", destRel, err)
			}
			var hash string
			if mode == core.ModeSymlink {
				target, err := os.Readlink(destFull)
				if err != nil {
					return fmt.Errorf("failed to read symlink %s: %w", destRel, err)
				}
				hash, err = a.store.PutBlob([]byte(target))
				if err != nil {
					return fmt.Errorf("failed to store symlink %s: %w", destRel, err)
				}
			} else {
				hash, err = a.wt.PutBlobForAdd(destFull)
				if err != nil {
					return fmt.Errorf("failed to store %s: %w", destRel, err)
				}
			}
			entry := core.IndexEntry{
				Path:       destRel,
				Hash:       hash,
				ModifiedAt: info.ModTime(),
				Size:       info.Size(),
				Mode:       mode,
			}
			if err := idx.Add(entry); err != nil {
				return fmt.Errorf("failed to stage %s: %w", destRel, err)
			}
		}
	}

	if err := a.store.SaveIndex(&idx); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	var srcDirs []string
	for _, src := range sources {
		// Best-effort: these functions rarely fail for normal paths.
		absSrc, _ := filepath.Abs(src)
		srcRel, _ := filepath.Rel(a.dir, absSrc)
		srcRel = filepath.ToSlash(srcRel)
		dir := filepath.Dir(srcRel)
		if dir != "." && dir != "" {
			srcDirs = append(srcDirs, dir)
		}
	}
	if len(srcDirs) > 0 {
		a.wt.CleanEmptyDirs(srcDirs)
	}

	return nil
}

func (a *App) Clean(opts CleanOptions) ([]string, error) {
	return a.wt.CleanUntracked(opts.Dirs, opts.DryRun)
}
