// Package pathutil provides cross-platform path utilities.
// All paths returned by this package use forward slashes,
// matching the normalized format stored in snapshots and indexes.
package pathutil

import (
	"path/filepath"
)

// Rel returns a normalized relative path (with / separators) from basepath to targpath.
// It combines filepath.Rel with filepath.ToSlash to ensure cross-platform consistency.
func Rel(basepath, targpath string) (string, error) {
	rel, err := filepath.Rel(basepath, targpath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// Normalize ensures a path uses forward slashes, matching the internal storage format.
// Use this for user-provided paths before comparing with stored paths.
func Normalize(p string) string {
	return filepath.ToSlash(p)
}
