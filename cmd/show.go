package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
)

// isVersionRef reports whether s looks like a snapshot version reference
// (id:..., tag:..., branch:..., or the bare keyword "head") rather than a
// file path. Used by `drift show` to disambiguate a single argument.
func isVersionRef(s string) bool {
	if s == "head" {
		return true
	}
	return strings.HasPrefix(s, "id:") ||
		strings.HasPrefix(s, "tag:") ||
		strings.HasPrefix(s, "branch:")
}

// Layout constants for showFileList output.
const (
	// minPathPadding is added to the longest path width so short paths
	// still align with the size column.
	minPathPadding = 3
	// sizeColWidth is the field width for the formatted size in the file
	// listing.
	sizeColWidth = 8
)

var showOpen bool

var showCmd = &cobra.Command{
	Use:   "show [<version>] [<file>]",
	Short: "Show snapshot contents or a file from a snapshot",
	Long: "Show lists files in a snapshot, or displays a file's content.\n" +
		"\n" +
		"Without arguments, shows help.\n" +
		"With only a version, lists files in that snapshot.\n" +
		"With a version and a file, displays the file content (text) or metadata (binary/image).",
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Show", "show", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if len(args) == 0 {
			return cmd.Help()
		}

		// Argument parsing:
	//   - 1 arg that looks like a version reference (starts with "id:",
	//     "tag:", "branch:", or equals "head"): list that snapshot's files.
	//   - 1 arg not matching a version prefix: treated as a file path with
	//     an implicit HEAD version. This is an intentional UX decision:
	//     `drift show README.md` reads more naturally than
	//     `drift show head README.md`. The bare-name-as-branch grammar
	//     still applies when the user intends a branch (e.g.
	//     `drift show main`).
	//   - 2 args: version then file.
	var versionLabel, filePath string
	if len(args) == 1 {
		if isVersionRef(args[0]) {
			versionLabel = args[0]
		} else {
			versionLabel = "head"
			filePath = args[0]
		}
	} else {
			versionLabel = args[0]
			filePath = args[1]
		}

		snapshot := resolveSnapshot(ctx, store, versionLabel)
	if snapshot == nil {
		reportFailed("Show", "show", fmt.Sprintf("snapshot not found: %s.", versionLabel),
			"use 'drift log' to list available snapshots.")
		return ErrSilent
	}

	if filePath == "" {
		return showFileList(ctx, store, snapshot, versionLabel)
	}
	return showFile(ctx, store, cwd, snapshot, versionLabel, filePath)
	},
}

// showFileList prints the list of files in a snapshot with type info.
// The status line uses versionLabel (the user-supplied reference) so the
// output matches the input syntax rather than the resolved short ID.
func showFileList(ctx context.Context, store storage.Storer, snap *core.Snapshot, versionLabel string) error {
	if globalJSON {
		return showFileListJSON(ctx, store, snap, versionLabel)
	}
	// Quiet mode: suppress file listing output.
	if globalQuiet {
		return nil
	}
	fmt.Printf(">>> Snapshot %s (%d %s)\n", versionLabel, len(snap.Files), pluralFile(len(snap.Files)))
	if len(snap.Files) == 0 {
		fmt.Println()
		fmt.Println("  (no files)")
		return nil
	}
	fmt.Println()

	pathWidth := 0
	for _, f := range snap.Files {
		if len(f.Path) > pathWidth {
			pathWidth = len(f.Path)
		}
	}
	// Minimum padding so short paths still align with the size column.
	pathWidth += minPathPadding

	for i := range snap.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := &snap.Files[i]
		typeLabel := porcelain.DetectFileTypeLabel(ctx, store, f)
		fmt.Printf("  %-*s%*s   %s\n", pathWidth, f.Path, sizeColWidth, formatSize(f.Size), typeLabel)
	}

	fmt.Println()
	fmt.Printf("  %d %s\n", len(snap.Files), pluralFile(len(snap.Files)))
	return nil
}

// showFile displays a single file from a snapshot: text content is streamed
// to stdout, binary/image files show metadata. versionLabel is used in the
// status line so the output matches the user's input reference.
func showFile(ctx context.Context, store storage.Storer, cwd string, snapshot *core.Snapshot, versionLabel, filePath string) error {
	if globalJSON {
		return showFileJSON(ctx, store, cwd, snapshot, versionLabel, filePath)
	}

	result, err := porcelain.ReadSnapshotFile(ctx, store, snapshot, cwd, filePath)
	if err != nil {
		if errors.Is(err, porcelain.ErrFileNotFound) {
			reportFailed("Show", "show", fmt.Sprintf("'%s' not found in snapshot %s.", filePath, versionLabel),
				fmt.Sprintf("use 'drift show %s' to list files in this snapshot.", versionLabel))
			return ErrSilent
		}
		if errors.Is(err, porcelain.ErrInvalidPath) {
			reportFailed("Show", "show", fmt.Sprintf("cannot resolve path '%s'.", filePath),
				"use a relative path from the project root.")
			return ErrSilent
		}
		reportFailed("Show", "show", fmt.Sprintf("cannot read '%s' from snapshot: %s.", filePath, err),
			"the chunk data may be missing or corrupted; use 'drift check' to verify.")
		return ErrSilent
	}

	if showOpen {
		return openExternal(versionLabel, filePath, bytes.NewReader(result.Content))
	}

	if globalQuiet {
		return nil
	}

	if result.Kind == "text" {
		fmt.Printf(">>> File %s:%s\n", versionLabel, filePath)
		fmt.Println()
		if _, err := os.Stdout.Write(result.Content); err != nil {
			reportFailed("Show", "show", fmt.Sprintf("failed to stream '%s': %s.", filePath, err), "")
			return ErrSilent
		}
		return nil
	}

	fmt.Printf(">>> File %s:%s\n", versionLabel, filePath)
	fmt.Printf("  Size:       %s\n", formatSize(result.Size))
	if result.Kind == "image" && result.Dimensions != "" {
		fmt.Printf("  Dimensions: %s\n", result.Dimensions)
	}
	if result.ModTime > 0 {
		modTimeStr := time.Unix(0, result.ModTime).Format("01-02 15:04")
		fmt.Printf("  Modified:   %s\n", modTimeStr)
	}
	fmt.Println()
	fmt.Println("  hint: use --open to view with system program.")
	return nil
}

func init() {
	showCmd.Flags().BoolVar(&showOpen, "open", false, "open file with system viewer")
	rootCmd.AddCommand(showCmd)
}
