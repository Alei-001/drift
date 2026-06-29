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

// RelToWorkDir converts a user-provided path to a normalized relative path from workDir.
// If the path is absolute, it is made relative to workDir. If relative, it is normalized.
// This is the standard entry point for user-facing file paths in CLI commands.
func RelToWorkDir(workDir, path string) (string, error) {
	path = Normalize(path)
	if filepath.IsAbs(path) {
		return Rel(workDir, path)
	}
	return path, nil
}
