package video

import (
	"bytes"
	"fmt"
)

// Diff compares two video files by size only. Video container internals are
// not parsed to avoid pulling in heavyweight decoding dependencies. Identical
// bytes (checked via bytes.Equal) short-circuit to an empty diff.
func (e *VideoEngine) Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error) {
	if bytes.Equal(oldData, newData) {
		return "", nil
	}
	oldSize := humanReadableSize(len(oldData))
	newSize := humanReadableSize(len(newData))
	return fmt.Sprintf("video file size changed: %s -> %s", oldSize, newSize), nil
}

// humanReadableSize formats a byte count as a human-friendly string using
// binary units (B, KB, MB, GB).
func humanReadableSize(n int) string {
	const (
		kb = 1024
		mb = 1024 * 1024
		gb = 1024 * 1024 * 1024
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%d GB", n/gb)
	case n >= mb:
		return fmt.Sprintf("%d MB", n/mb)
	case n >= kb:
		return fmt.Sprintf("%d KB", n/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
