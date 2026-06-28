package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/your-org/drift/core"
)

// -- Status line helpers --

// statusOK prints ">>> <format> [ok]" to stdout.
func statusOK(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf(">>> %s [ok]\n", msg)
}

// statusActive prints ">>> <format> [active]" to stdout.
func statusActive(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf(">>> %s [active]\n", msg)
}

// statusWarn prints ">>> <format> [warning]" to stdout.
func statusWarn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf(">>> %s [warning]\n", msg)
}

// statusFailed prints the error block: status line + Error + hint.
func statusFailed(action string, errMsg string, hint string) {
	fmt.Printf(">>> %s [failed]\n", action)
	fmt.Printf("Error: %s\n", errMsg)
	if hint != "" {
		fmt.Printf("  hint: %s\n", hint)
	}
}

// statusInactive prints ">>> <format> [inactive]" to stdout.
func statusInactive(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf(">>> %s [inactive]\n", msg)
}

// -- File list formatting --

// fileChar returns the status symbol for a file change direction.
// added=true means the file was added, deleted=true means deleted, otherwise modified.
func fileChar(added, deleted bool) string {
	if added {
		return "+"
	}
	if deleted {
		return "-"
	}
	return "~"
}

// printFileList prints a file list with status symbols and optional sizes.
// Each file entry gets "  <char>  <path>  <size>" format.
// Returns counts of added, modified, deleted.
func printFileList(titleFmt string, titleArgs []interface{}, addedFiles []core.FileEntry, deletedPaths []string, modFiles []core.FileEntry) (added, mod, del int) {
	if len(addedFiles)+len(deletedPaths)+len(modFiles) == 0 {
		return 0, 0, 0
	}

	fmt.Println()
	for _, f := range addedFiles {
		fmt.Printf("  +  %s\n", f.Path)
		// Print size on the next line for compactness, or on same line? Design doc shows:
		//   +  chapter4.md      12.3 KB
		// Let's keep it simple: path only in diff, path+size in save/restore/verbose
	}
	for _, f := range modFiles {
		fmt.Printf("  ~  %s\n", f.Path)
	}
	for _, p := range deletedPaths {
		fmt.Printf("  -  %s\n", p)
	}

	return len(addedFiles), len(modFiles), len(deletedPaths)
}

// printFileListWithSize prints file list with sizes (for save/restore).
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

// printFileListWithLineCount prints file list with sizes and line counts (for log -v).
func printFileListWithLineCount(added, modified []core.FileEntry, deleted []string, store interface{ GetChunk(core.Hash) (*core.Chunk, error) }) {
	fmt.Println()
	for _, f := range added {
		fmt.Printf("  +  %s      %s\n", f.Path, formatSize(f.Size))
	}
	for _, f := range modified {
		lines := countLinesFromChunks(store, f)
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
func countLinesFromChunks(store interface{ GetChunk(core.Hash) (*core.Chunk, error) }, entry core.FileEntry) int {
	var data []byte
	for _, h := range entry.Chunks {
		chunk, err := store.GetChunk(h)
		if err != nil {
			return 0
		}
		data = append(data, chunk.Data...)
	}
	return strings.Count(string(data), "\n")
}

// summaryLine prints "  N files: +A ~M -D".
func summaryLine(total, added, mod, del int) {
	fmt.Printf("\n  %d files: +%d ~%d -%d\n", total, added, mod, del)
}

// -- Error helpers --

// fatalNoRepo is a helper for "not a drift repository" errors.
func fatalNoRepo() {
	statusFailed("Init", "not a drift repository.", "use 'drift init' to create one.")
	os.Exit(1)
}

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

// chunkHashesEqual compares two slices of hashes for equality.
func chunkHashesEqual(a, b []core.Hash) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d second(s)", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%d minute(s)", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%d hour(s)", int(d.Hours()))
	}
	return fmt.Sprintf("%d day(s)", int(d.Hours()/24))
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
