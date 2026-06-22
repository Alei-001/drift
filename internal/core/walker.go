package core

import (
	"os"
	"path/filepath"
	"strings"
)

type WalkFunc func(path string, info os.FileInfo) error

func WalkWorkingDir(root string, fn WalkFunc) error {
	ignore := LoadDriftIgnore(root)

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
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

		if ignore.IsIgnored(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
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
