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
	Force  bool
	DryRun bool
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
		return nil, nil
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

func (a *App) Move(sources []string, dest string, opts MoveOptions) ([]string, error) {
	destInfo, destErr := os.Lstat(filepath.Join(a.dir, filepath.FromSlash(dest)))
	destIsDir := destErr == nil && destInfo.IsDir()

	if !destIsDir && len(sources) > 1 {
		return nil, fmt.Errorf("moving multiple sources requires the destination to be a directory")
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
		return nil, fmt.Errorf("failed to load parent tree: %w", err)
	}
	for p := range parentHashes {
		tracked[p] = true
	}

	// Phase 1: validate all sources before moving anything. This prevents
	// partial-failure inconsistency where some files are moved on disk but
	// the index hasn't been saved.
	type plannedMove struct {
		srcRel   string
		destRel  string
		srcFull  string
		destFull string
	}

	var planned []plannedMove
	for _, src := range sources {
		absSrc, err := filepath.Abs(src)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve source %q: %w", src, err)
		}
		srcRel, err := filepath.Rel(a.dir, absSrc)
		if err != nil {
			return nil, fmt.Errorf("cannot make %q relative to worktree: %w", src, err)
		}
		srcRel = filepath.ToSlash(srcRel)
		if !tracked[srcRel] {
			return nil, fmt.Errorf("source '%s' is not tracked (use 'drift add' first)", srcRel)
		}

		var destRel string
		if destIsDir {
			destRel = filepath.ToSlash(filepath.Join(dest, filepath.Base(srcRel)))
		} else {
			destRel = filepath.ToSlash(dest)
		}

		if err := core.ValidateTreePath(destRel); err != nil {
			return nil, fmt.Errorf("invalid destination %q: %w", destRel, err)
		}

		srcFull := filepath.Join(a.dir, filepath.FromSlash(srcRel))
		destFull := filepath.Join(a.dir, filepath.FromSlash(destRel))

		if _, err := os.Stat(destFull); err == nil {
			if !opts.Force {
				return nil, fmt.Errorf("destination exists (use -f to overwrite): %s", destRel)
			}
		}

		// Check for duplicate destinations among sources.
		for _, p := range planned {
			if p.destRel == destRel {
				return nil, fmt.Errorf("multiple sources would move to the same destination: %s", destRel)
			}
		}

		planned = append(planned, plannedMove{
			srcRel:   srcRel,
			destRel:  destRel,
			srcFull:  srcFull,
			destFull: destFull,
		})
	}

	// DryRun: return planned destinations without executing moves.
	if opts.DryRun {
		var dests []string
		for _, p := range planned {
			dests = append(dests, p.destRel)
		}
		return dests, nil
	}

	// Phase 2: execute all moves. By this point all preconditions are validated.
	var moved []string
	for _, p := range planned {
		if err := os.MkdirAll(filepath.Dir(p.destFull), 0755); err != nil {
			return moved, fmt.Errorf("failed to create destination directory: %w", err)
		}

		if _, err := os.Stat(p.destFull); err == nil {
			if err := os.Remove(p.destFull); err != nil {
				return moved, fmt.Errorf("failed to remove existing destination %s: %w", p.destRel, err)
			}
		}

		if err := os.Rename(p.srcFull, p.destFull); err != nil {
			return moved, fmt.Errorf("failed to move %s to %s: %w", p.srcRel, p.destRel, err)
		}

		if entry, err := idx.Entry(p.srcRel); err == nil {
			newEntry := entry
			newEntry.Path = p.destRel
			if err := idx.Add(newEntry); err != nil {
				return moved, fmt.Errorf("failed to stage %s: %w", p.destRel, err)
			}
			idx.Remove(p.srcRel)
		} else {
			hash, mode, storeErr := a.wt.StoreBlob(p.destFull)
			if storeErr != nil {
				return moved, fmt.Errorf("failed to store %s: %w", p.destRel, storeErr)
			}
			info, statErr := os.Lstat(p.destFull)
			entry := core.IndexEntry{
				Path: p.destRel,
				Hash: hash,
				Mode: mode,
			}
			if statErr == nil {
				entry.ModifiedAt = info.ModTime()
				entry.Size = info.Size()
			}
			if err := idx.Add(entry); err != nil {
				return moved, fmt.Errorf("failed to stage %s: %w", p.destRel, err)
			}
		}
		moved = append(moved, p.destRel)
	}

	if err := a.store.SaveIndex(&idx); err != nil {
		return moved, fmt.Errorf("failed to save index: %w", err)
	}

	var srcDirs []string
	for _, p := range planned {
		dir := filepath.Dir(p.srcRel)
		if dir != "." && dir != "" {
			srcDirs = append(srcDirs, dir)
		}
	}
	if len(srcDirs) > 0 {
		a.wt.CleanEmptyDirs(srcDirs)
	}

	return moved, nil
}

func (a *App) Clean(opts CleanOptions) ([]string, error) {
	return a.wt.CleanUntracked(opts.Dirs, opts.DryRun)
}
