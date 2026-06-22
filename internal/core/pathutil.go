package core

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// ErrInvalidPath is returned when a path contains traversal or absolute components
// that would escape the working directory, or disguised .drift metadata names.
var ErrInvalidPath = errors.New("invalid path")

// hfsIgnoredCodepoints contains Unicode code points that HFS+ ignores
// during path normalization. A path component containing these characters
// between the bytes of ".drift" (or ".git", etc.) will be treated as that
// name by HFS+, so they must be filtered out before comparison.
//
// Mirrors go-git's pathutil.hfsIgnoredCodepoints (upstream Git utf8.c).
var hfsIgnoredCodepoints = map[rune]struct{}{
	0x200c: {}, // ZERO WIDTH NON-JOINER
	0x200d: {}, // ZERO WIDTH JOINER
	0x200e: {}, // LEFT-TO-RIGHT MARK
	0x200f: {}, // RIGHT-TO-LEFT MARK
	0x202a: {}, // LEFT-TO-RIGHT EMBEDDING
	0x202b: {}, // RIGHT-TO-LEFT EMBEDDING
	0x202c: {}, // POP DIRECTIONAL FORMATTING
	0x202d: {}, // LEFT-TO-RIGHT OVERRIDE
	0x202e: {}, // RIGHT-TO-LEFT OVERRIDE
	0x206a: {}, // INHIBIT SYMMETRIC SWAPPING
	0x206b: {}, // ACTIVATE SYMMETRIC SWAPPING
	0x206c: {}, // INHIBIT ARABIC FORM SHAPING
	0x206d: {}, // ACTIVATE ARABIC FORM SHAPING
	0x206e: {}, // NATIONAL DIGIT SHAPES
	0x206f: {}, // NOMINAL DIGIT SHAPES
	0xfeff: {}, // ZERO WIDTH NO-BREAK SPACE
}

// ValidateTreePath checks that a path is safe to use as a tree/index entry path.
// It rejects absolute paths, ".." traversal, empty segments, control characters,
// and .drift/.git metadata disguises (including HFS+/NTFS variants) — mirroring
// go-git's pathutil.ValidTreePath.
//
// Drift's metadata directory is ".drift" (not ".git"), so we reject both
// ".drift" and ".git" variants to stay safe when importing/exporting across
// ecosystems.
func ValidateTreePath(p string) error {
	if p == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidPath)
	}

	// Reject control characters (< 0x20, 0x7f).
	for i := 0; i < len(p); i++ {
		if p[i] < 0x20 || p[i] == 0x7f {
			return fmt.Errorf("%w %q: contains control character", ErrInvalidPath, p)
		}
	}

	// Normalize to forward slashes for consistent checking.
	p = filepath.ToSlash(p)

	if filepath.IsAbs(p) {
		return fmt.Errorf("%w %q: absolute path", ErrInvalidPath, p)
	}

	// Reject Windows volume name prefixes (e.g. C:).
	if vol := filepath.VolumeName(p); vol != "" {
		return fmt.Errorf("%w %q: contains volume name", ErrInvalidPath, p)
	}

	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '\\' || r == '/' })
	if len(parts) == 0 {
		return fmt.Errorf("%w: %q", ErrInvalidPath, p)
	}

	for _, part := range parts {
		if part == "." || part == ".." {
			return fmt.Errorf("%w %q: cannot use %q", ErrInvalidPath, p, part)
		}
		if part == "" {
			return fmt.Errorf("%w %q: empty segment", ErrInvalidPath, p)
		}

		// Reject .drift and .git metadata disguises at every path position.
		if IsDotGitName(part) || IsHFSDotGit(part) || IsNTFSDotGit(part) {
			return fmt.Errorf("%w component: %q (metadata disguise)", ErrInvalidPath, p)
		}
		if isDriftMetaName(part) {
			return fmt.Errorf("%w component: %q (drift metadata)", ErrInvalidPath, p)
		}
	}

	return nil
}

// IsDotGitName reports whether name is `.git` or its 8.3 NTFS short
// alias `git~1`, case-insensitively. Mirrors go-git's pathutil.IsDotGitName.
func IsDotGitName(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "git~1":
		return true
	}
	return false
}

// isDriftMetaName reports whether name is ".drift" or its likely aliases,
// case-insensitively. This prevents planting files that collide with Drift's
// own metadata directory.
func isDriftMetaName(name string) bool {
	switch strings.ToLower(name) {
	case ".drift", "drift~1":
		return true
	}
	return false
}

// IsHFSDot reports whether part would be treated as ".<needle>" on an
// HFS+ filesystem after stripping ignored Unicode code points and
// folding ASCII to lower case. Mirrors go-git's pathutil.IsHFSDot.
func IsHFSDot(part, needle string) bool {
	runes := []rune(part)
	i := 0

	// skip ignored code points, then expect '.'
	for i < len(runes) {
		if _, ok := hfsIgnoredCodepoints[runes[i]]; !ok {
			break
		}
		i++
	}
	if i >= len(runes) || runes[i] != '.' {
		return false
	}
	i++

	// match needle case-insensitively, skipping ignored code points
	for _, expected := range needle {
		for i < len(runes) {
			if _, ok := hfsIgnoredCodepoints[runes[i]]; !ok {
				break
			}
			i++
		}
		if i >= len(runes) {
			return false
		}
		r := runes[i]
		if r > 127 {
			return false
		}
		if unicode.ToLower(r) != expected {
			return false
		}
		i++
	}

	// skip trailing ignored code points
	for i < len(runes) {
		if _, ok := hfsIgnoredCodepoints[runes[i]]; !ok {
			break
		}
		i++
	}

	return i == len(runes)
}

// IsHFSDotGit reports whether part is an HFS+ equivalent of ".git".
func IsHFSDotGit(part string) bool { return IsHFSDot(part, "git") }

// IsNTFSDotGit detects path components that NTFS would resolve to ".git":
// the canonical name itself and its 8.3 short-name alias "git~1", each
// followed by any number of trailing spaces or periods (which NTFS silently
// trims) and an optional Alternate Data Stream suffix (":<stream>").
// Mirrors go-git's pathutil.IsNTFSDotGit.
func IsNTFSDotGit(part string) bool {
	var i int
	switch {
	case len(part) >= 4 && part[0] == '.' &&
		asciiToLower(part[1]) == 'g' &&
		asciiToLower(part[2]) == 'i' &&
		asciiToLower(part[3]) == 't':
		i = 4
	case len(part) >= 5 &&
		asciiToLower(part[0]) == 'g' &&
		asciiToLower(part[1]) == 'i' &&
		asciiToLower(part[2]) == 't' &&
		part[3] == '~' && part[4] == '1':
		i = 5
	default:
		return false
	}

	for ; i < len(part); i++ {
		c := part[i]
		if c == ':' {
			return true
		}
		if c != '.' && c != ' ' {
			return false
		}
	}
	return true
}

func asciiToLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// ErrSymlinkEscape is returned when a symlink target would resolve outside
// the repository root, which could be used to read or write arbitrary files
// during restore/switch operations.
var ErrSymlinkEscape = errors.New("symlink target escapes repository root")

// ValidateSymlinkTarget reports whether a symlink with the given target,
// created at linkPath within root, would resolve to a path inside root.
// Both root and linkPath are absolute or root-relative; target is the
// symlink's stored target string (may be relative or absolute).
//
// Mirrors go-git's filesystem checkSafe / symlink containment logic.
func ValidateSymlinkTarget(root, linkPath, target string) error {
	// Normalize to absolute paths for containment checking.
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return err
	}

	// The directory containing the symlink is the resolution base.
	linkDir := filepath.Dir(linkPath)
	absLinkDir, err := filepath.Abs(filepath.Join(absRoot, linkDir))
	if err != nil {
		return err
	}

	// Resolve the target relative to the link's directory.
	var resolved string
	if filepath.IsAbs(target) {
		resolved = filepath.Clean(target)
	} else {
		resolved = filepath.Clean(filepath.Join(absLinkDir, target))
	}

	// The resolved path must be within absRoot.
	rel, err := filepath.Rel(absRoot, resolved)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q resolves to %q (outside %s)",
			ErrSymlinkEscape, target, resolved, absRoot)
	}
	return nil
}
