package worktree

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
)

// NormalizePathFilters converts raw path arguments into normalized,
// repository-relative filter strings (no trailing slash, forward slashes).
// rootDir is the repository root used to compute repository-relative paths
// (independent of process cwd). Returns nil if no arguments are given.
func NormalizePathFilters(rootDir string, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	filters := make([]string, 0, len(args))
	for _, f := range args {
		absPath, err := filepath.Abs(f)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve relative path %q: %w", f, err)
		}
		rel, err := filepath.Rel(rootDir, absPath)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve relative path %q: %w", f, err)
		}
		filters = append(filters, strings.TrimSuffix(filepath.ToSlash(rel), "/"))
	}
	return filters, nil
}

// PathMatchesAny reports whether the given path matches any of the filters.
// A path matches if it equals a filter or is a descendant of it (prefix+"/").
// The special filter "." matches everything.
// Empty/nil filters means "match all" (no filtering).
func PathMatchesAny(path string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, fp := range filters {
		if fp == "." {
			return true
		}
		if path == fp || strings.HasPrefix(path, fp+"/") {
			return true
		}
	}
	return false
}

// FilterBlobs returns only the blob entries whose path matches any filter.
// If filters is nil/empty, all blobs are returned unchanged.
func FilterBlobs(blobs []core.BlobEntry, filters []string) []core.BlobEntry {
	if len(filters) == 0 {
		return blobs
	}
	var filtered []core.BlobEntry
	for _, b := range blobs {
		if PathMatchesAny(b.Path, filters) {
			filtered = append(filtered, b)
		}
	}
	return filtered
}
