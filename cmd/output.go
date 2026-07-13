package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/format"
)

// ErrSilent indicates that an error was already displayed to the user
// via statusFailed, and Execute() should exit with code 1 without
// printing the error again.
var ErrSilent = errors.New("silent error (already reported)")

// silentWrap returns an error that wraps both ErrSilent and err, so
// classifyError can still match the underlying sentinel (ErrNotARepo,
// ErrNetwork, etc.) via errors.Is. Use this instead of returning bare
// ErrSilent when the original error type matters for exit-code selection.
func silentWrap(err error) error {
	return fmt.Errorf("%w: %w", ErrSilent, err)
}

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

// openProjectOrReport opens the drift project at cwd. On failure it reports
// the error via reportFailed (JSON-aware) and returns ErrSilent so the caller
// can `return ErrSilent` directly. On success it returns the store and cfg.
// command is the JSON "command" field value (e.g. "log", "branch").
func openProjectOrReport(action, command, cwd string) (storage.Storer, *core.Config, error) {
	store, cfg, err := porcelain.OpenProject(cwd)
	if err != nil {
		reportFailed(action, command, "not a drift repository.", "use 'drift init' to create one.")
		return nil, nil, silentWrap(err)
	}
	return store, cfg, nil
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
func printFileListWithLineCount(added, modified []core.FileEntry, deleted []string, store storage.Storer) {
	ctx := context.Background()
	fmt.Println()
	for _, f := range added {
		fmt.Printf("  +  %s      %s\n", f.Path, formatSize(f.Size))
	}
	for _, f := range modified {
		lines := porcelain.CountFileLines(ctx, store, f)
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
	return format.Bytes(size)
}
