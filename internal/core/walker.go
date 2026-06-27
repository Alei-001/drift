package core

import (
	"os"
	"path/filepath"
	"strings"
)

type WalkFunc func(path string, info os.FileInfo) error

func WalkWorkingDir(root string, fn WalkFunc) error {
	return WalkWorkingDirWithIgnore(root, root, fn)
}

// WalkWorkingDirWithIgnore walks walkRoot but loads ignore patterns from ignoreRoot.
// This allows adding a subdirectory while still respecting the project-root .driftignore.
func WalkWorkingDirWithIgnore(walkRoot, ignoreRoot string, fn WalkFunc) error {
	ignore := LoadDriftIgnore(ignoreRoot)
	walkAbs, err := filepath.Abs(walkRoot)
	if err != nil {
		return err
	}
	ignoreAbs, err := filepath.Abs(ignoreRoot)
	if err != nil {
		return err
	}

	return filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(walkRoot, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)

		if shouldSkip(rel, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// For ignore matching, use the path relative to ignoreRoot so that
		// project-root .driftignore patterns work when walking a subdirectory.
		ignoreRel := rel
		if walkAbs != ignoreAbs {
			if r, err := filepath.Rel(ignoreAbs, path); err == nil {
				ignoreRel = filepath.ToSlash(r)
			}
		}

		if info.IsDir() {
			if ignore.IsIgnoredDir(ignoreRel) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignore.IsIgnored(ignoreRel) {
			return nil
		}

		return fn(rel, info)
	})
}

func shouldSkip(relPath string, info os.FileInfo) bool {
	if relPath == ".drift" || strings.HasPrefix(relPath, ".drift/") {
		return true
	}
	if relPath == ".git" || strings.HasPrefix(relPath, ".git/") {
		return true
	}
	return false
}
