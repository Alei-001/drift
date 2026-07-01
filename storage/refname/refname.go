// Package refname validates reference names (branch, tag, HEAD) for both
// storage backends. Centralizing this logic prevents divergence between
// the filesystem and memory backends.
package refname

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/your-org/drift/storage"
)

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
	return nil
}

// IsWindowsReservedName checks if a name is reserved by Windows and cannot
// be used as a filename. Covers CON, AUX, NUL, PRN, COM0-9, LPT0-9.
func IsWindowsReservedName(name string) bool {
	switch name {
	case "con", "aux", "nul", "prn":
		return true
	}
	if len(name) >= 4 {
		switch name[:3] {
		case "com", "lpt":
			// Windows reserves COM0-COM9 and LPT0-LPT9 on modern builds.
			if name[3] >= '0' && name[3] <= '9' {
				return true
			}
		}
	}
	return false
}
