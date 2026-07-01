package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"github.com/your-org/drift/core"
)

// ErrSilent indicates that an error was already displayed to the user
// via statusFailed, and Execute() should exit with code 1 without
// printing the error again.
var ErrSilent = errors.New("silent error (already reported)")

// -- Status line helpers --

// statusOK prints ">>> <format> [ok]" to stdout.
func statusOK(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf(">>> %s [ok]\n", msg)
}

// statusWarn prints ">>> <format> [warning]" to stdout.
func statusWarn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf(">>> %s [warning]\n", msg)
}

// statusActive prints ">>> <format> [active]" to stdout.
func statusActive(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf(">>> %s [active]\n", msg)
}

// statusFailed prints the error block: status line + Error + hint.
func statusFailed(action string, errMsg string, hint string) {
	fmt.Fprintf(os.Stderr, ">>> %s [failed]\n", action)
	fmt.Fprintf(os.Stderr, "Error: %s\n", errMsg)
	if hint != "" {
		fmt.Fprintf(os.Stderr, "  hint: %s\n", hint)
	}
}

// -- File list formatting --

// printFileListWithSize prints file list with sizes (for save).
func printFileListWithSize(added, modified []core.FileEntry, deleted []string) {
	fmt.Println()
	for _, f := range added {
		fmt.Printf("  +  %s      %s\n", f.Path, formatSize(f.Size))
	}
	for _, f := range modified {
		fmt.Printf("  ~  %s      %s\n", f.Path, formatSize(f.Size))
	}
	for _, p := range deleted {
		fmt.Printf("  -  %s\n", p)
	}
}

// printFileListSimple prints file list without sizes (for restore).
func printFileListSimple(added, modified []core.FileEntry, deleted []string) {
	fmt.Println()
	for _, f := range added {
		fmt.Printf("  +  %s\n", f.Path)
	}
	for _, f := range modified {
		fmt.Printf("  ~  %s\n", f.Path)
	}
	for _, p := range deleted {
		fmt.Printf("  -  %s\n", p)
	}
}

// printFileListWithLineCount prints file list with sizes and line counts (for log -v).
func printFileListWithLineCount(added, modified []core.FileEntry, deleted []string, store interface {
	GetChunk(context.Context, core.Hash) (*core.Chunk, error)
}) {
	ctx := context.Background()
	fmt.Println()
	for _, f := range added {
		fmt.Printf("  +  %s      %s\n", f.Path, formatSize(f.Size))
	}
	for _, f := range modified {
		lines := countLinesFromChunks(ctx, store, f)
		if lines > 0 {
			fmt.Printf("  ~  %s      %s  (%d lines)\n", f.Path, formatSize(f.Size), lines)
		} else {
			fmt.Printf("  ~  %s      %s\n", f.Path, formatSize(f.Size))
		}
	}
	for _, p := range deleted {
		fmt.Printf("  -  %s\n", p)
	}
}

// countLinesFromChunks reads chunk data and counts newlines.
func countLinesFromChunks(ctx context.Context, store interface {
	GetChunk(context.Context, core.Hash) (*core.Chunk, error)
}, entry core.FileEntry) int {
	var data []byte
	for _, h := range entry.Chunks {
		chunk, err := store.GetChunk(ctx, h)
		if err != nil {
			return 0
		}
		data = append(data, chunk.Data...)
	}
	return strings.Count(string(data), "\n")
}

// summaryLine prints "  N files: +A ~M -D", omitting zero-count parts.
// Example: 3 files: +2 ~1   (no "-0" when there are no deletions)
func summaryLine(total, added, mod, del int) {
	parts := []string{}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if mod > 0 {
		parts = append(parts, fmt.Sprintf("~%d", mod))
	}
	if del > 0 {
		parts = append(parts, fmt.Sprintf("-%d", del))
	}
	if len(parts) == 0 {
		parts = append(parts, "+0")
	}
	fmt.Printf("\n  %d %s: %s\n", total, pluralFile(total), strings.Join(parts, " "))
}

// pluralFile returns "file" or "files" depending on n.
func pluralFile(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}

// -- Error helpers --

// formatSize converts bytes to a human-readable string.
func formatSize(size int64) string {
	switch {
	case size < 0:
		return fmt.Sprintf("-%s", formatSize(-size))
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
}

func parseHexByte(s string) (byte, bool) {
	if len(s) != 2 {
		return 0, false
	}
	var b byte
	for i := 0; i < 2; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			b = b<<4 | (c - '0')
		case c >= 'a' && c <= 'f':
			b = b<<4 | (c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			b = b<<4 | (c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return b, true
}

// imageDimensions decodes image dimensions from data for common image formats
// (PNG, JPEG, GIF). Returns empty string for non-image or undecodable data.
func imageDimensions(data []byte) string {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%dx%d", cfg.Width, cfg.Height)
}
