// Package pathutil provides cross-platform path utilities.
// All paths returned by this package use forward slashes,
// matching the normalized format stored in snapshots and indexes.
package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"
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

// Normalize ensures a path uses forward slashes and is cleaned of . and ..
// components, matching the internal storage format.
// Use this for user-provided paths before comparing with stored paths.
func Normalize(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}

// RelToWorkDir converts a user-provided path to a normalized relative path from workDir.
// If the path is absolute, it is made relative to workDir. If relative, it is normalized.
// Returns an error if the resulting path would escape workDir (contains "..").
// This is the standard entry point for user-facing file paths in CLI commands.
func RelToWorkDir(workDir, path string) (string, error) {
	path = Normalize(path)
	if filepath.IsAbs(path) {
		rel, err := Rel(workDir, path)
		if err != nil {
			return "", err
		}
		path = rel
	}
	// Reject Unix-style rooted paths that filepath.IsAbs misses on Windows
	// (e.g. "/etc/passwd", "\foo"). These are absolute on Unix and should
	// be rejected on all platforms for consistency.
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return "", fmt.Errorf("path escapes workspace root: %s", path)
	}
	// Reject paths that escape the workspace root after cleaning.
	// This covers both relative inputs like "../foo" and absolute inputs
	// outside workDir, which filepath.Rel turns into "../foo".
	if path == ".." || strings.HasPrefix(path, "../") {
		return "", fmt.Errorf("path escapes workspace root: %s", path)
	}
	return path, nil
}
