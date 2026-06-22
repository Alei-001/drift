package core

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrInvalidPath is returned when a path contains traversal or absolute components
// that would escape the working directory.
var ErrInvalidPath = errors.New("invalid path: contains traversal or absolute components")

// ValidateTreePath checks that a path is safe to use as a tree/index entry path.
// It rejects absolute paths, ".." traversal, and empty segments — mirroring
// go-git's pathutil.ValidTreePath. The path must be relative and stay within
// the working directory.
func ValidateTreePath(p string) error {
	if p == "" {
		return ErrInvalidPath
	}

	// Normalize to forward slashes for consistent checking.
	p = filepath.ToSlash(p)

	if filepath.IsAbs(p) {
		return ErrInvalidPath
	}

	// Reject Windows drive prefixes like "C:".
	if len(p) >= 2 && p[1] == ':' && (p[0] >= 'A' && p[0] <= 'Z' || p[0] >= 'a' && p[0] <= 'z') {
		return ErrInvalidPath
	}

	// Reject any ".." segment.
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return ErrInvalidPath
		}
		if seg == "" {
			return ErrInvalidPath
		}
	}

	return nil
}
