// Package refname validates reference names (branch, tag, HEAD) for both
// storage backends. Centralizing this logic prevents divergence between
// the filesystem and memory backends.
package refname

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// headsPrefix and tagsPrefix are the on-disk ref-name prefixes used by both
// backends. They mirror filesystem.HeadsDir / filesystem.TagsDir but are
// duplicated here so refname does not depend on a concrete backend package
// (which would be a layering violation — backends import refname, not vice
// versa).
const (
	headsPrefix = "heads/"
	tagsPrefix  = "tags/"
)

// RefType derives the core.RefType from a reference name. "HEAD" maps to
// RefTypeHead; names prefixed with "heads/" or "tags/" map to RefTypeBranch
// or RefTypeTag respectively; anything else defaults to RefTypeBranch. This
// logic is shared by the filesystem and memory backends so both return the
// same Type for the same name without duplicating the dispatch.
func RefType(name string) core.RefType {
	if name == "HEAD" {
		return core.RefTypeHead
	}
	if strings.HasPrefix(name, headsPrefix) {
		return core.RefTypeBranch
	}
	if strings.HasPrefix(name, tagsPrefix) {
		return core.RefTypeTag
	}
	return core.RefTypeBranch
}

// reservedBareNames lists bare names that cannot be used as branch or tag
// names because they are reserved as snapshot reference keywords (see
// porcelain.ResolveSnapshotRef). The check is case-insensitive to avoid
// ambiguity on case-insensitive filesystems (Windows, macOS default).
//
// Only "head" is reserved as a standalone keyword. The "id", "tag", and
// "branch" prefixes require a colon (which is already rejected by the
// character loop below), so bare names like "id", "tag", "branch" remain
// valid branch names — they only take on keyword meaning when followed
// by a colon.
var reservedBareNames = map[string]bool{
	"head": true,
}

// Validate validates a reference name. Returns nil if valid.
func Validate(name string) error {
	if name == "" {
		return fmt.Errorf("ref name is empty: %w", storage.ErrInvalidRef)
	}
	if name == "HEAD" {
		return nil
	}
	for _, c := range name {
		if c < 0x20 || c == 0x7F {
			return fmt.Errorf("invalid ref name: %q contains control character: %w", name, storage.ErrInvalidRef)
		}
		if c == ' ' {
			return fmt.Errorf("invalid ref name: %q contains space: %w", name, storage.ErrInvalidRef)
		}
		if c == '\\' || c == ':' {
			return fmt.Errorf("invalid ref name: %q contains %q: %w", name, string(c), storage.ErrInvalidRef)
		}
	}
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid ref name: %q starts with '-' or '/': %w", name, storage.ErrInvalidRef)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid ref name: %q contains '..': %w", name, storage.ErrInvalidRef)
	}
	base := strings.ToLower(filepath.Base(name))
	if IsWindowsReservedName(base) {
		return fmt.Errorf("invalid ref name: %q is a reserved name: %w", name, storage.ErrInvalidRef)
	}
	// Reject reserved bare names (case-insensitive on the base name, e.g.
	// "heads/head" → base "head") so they remain available as snapshot
	// reference keywords. "HEAD" itself is allowed (system ref, handled above).
	if reservedBareNames[base] {
		return fmt.Errorf("invalid ref name: %q is a reserved keyword: %w", name, storage.ErrInvalidRef)
	}
	return nil
}

// IsWindowsReservedName checks if a name is reserved by Windows and cannot
// be used as a filename. Covers CON, AUX, NUL, PRN, COM0-9, LPT0-9.
// The check is case-insensitive, matching Windows filesystem behavior.
func IsWindowsReservedName(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case "con", "aux", "nul", "prn":
		return true
	}
	// COM0-COM9 and LPT0-LPT9 are reserved as exactly 4-character names
	// (3-letter prefix + single digit). Multi-digit suffixes like "com10"
	// or "lpt99" are NOT reserved on Windows and must not match here.
	if len(name) == 4 {
		switch name[:3] {
		case "com", "lpt":
			if name[3] >= '0' && name[3] <= '9' {
				return true
			}
		}
	}
	return false
}
