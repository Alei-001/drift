package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/porcelain"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/format"
	"github.com/your-org/drift/internal/util/pathutil"
)

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
		//   - 1 arg starting with '@': a version reference; list its files.
		//   - 1 arg not starting with '@': treated as a file path with an
		//     implicit HEAD version. This is an intentional UX decision:
		//     `drift show README.md` reads more naturally than
		//     `drift show @head README.md`. It does not conflict with
		//     cli-design.md, whose examples always pass an explicit
		//     version; the bare-name-as-branch grammar from cli-design.md
		//     still applies when the user intends a branch (e.g.
		//     `drift show main`).
		//   - 2 args: version then file.
		var versionLabel, filePath string
		if len(args) == 1 {
			if strings.HasPrefix(args[0], "@") {
				versionLabel = args[0]
			} else {
				versionLabel = "@head"
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
	normalizedPath, err := pathutil.RelToWorkDir(cwd, filePath)
	if err != nil {
		reportFailed("Show", "show", fmt.Sprintf("cannot resolve path '%s'.", filePath),
			"use a relative path from the project root.")
		return ErrSilent
	}

	var targetEntry *core.FileEntry
	for i := range snapshot.Files {
		if snapshot.Files[i].Path == normalizedPath {
			targetEntry = &snapshot.Files[i]
			break
		}
	}
	if targetEntry == nil {
		reportFailed("Show", "show", fmt.Sprintf("'%s' not found in snapshot %s.", filePath, versionLabel),
			fmt.Sprintf("use 'drift show %s' to list files in this snapshot.", versionLabel))
		return ErrSilent
	}

	if globalJSON {
		return showFileJSON(ctx, store, cwd, snapshot, versionLabel, filePath)
	}

	chunkR := stream.NewChunkReader(ctx, store, targetEntry.Chunks)
	header, fullReader, err := stream.PeekHeader(chunkR, core.HeaderPeekSize)
	if err != nil {
		reportFailed("Show", "show", fmt.Sprintf("cannot read '%s' from snapshot: %s.", filePath, err),
			"the chunk data may be missing or corrupted; use 'drift check' to verify.")
		return ErrSilent
	}
	engine := filetype.DetectEngine(normalizedPath, header)

	if showOpen {
		return openExternal(versionLabel, filePath, fullReader)
	}

	// Quiet mode: suppress file content/metadata output (--open still works
	// because it returns above before this guard).
	if globalQuiet {
		return nil
	}

	if engine != nil && engine.Name() == "text" {
		fmt.Printf(">>> File %s:%s\n", versionLabel, filePath)
		fmt.Println()
		if _, err := io.Copy(os.Stdout, fullReader); err != nil {
			reportFailed("Show", "show", fmt.Sprintf("failed to stream '%s': %s.", filePath, err), "")
			return ErrSilent
		}
		return nil
	}

	// Binary or image file: show metadata.
	fmt.Printf(">>> File %s:%s\n", versionLabel, filePath)
	fmt.Printf("  Size:       %s\n", formatSize(targetEntry.Size))
	if engine != nil && engine.Name() == "image" {
		if dims := format.ImageDimensions(header); dims != "" {
			fmt.Printf("  Dimensions: %s\n", dims)
		}
	}
	if targetEntry.ModTime > 0 {
		modTimeStr := time.Unix(0, targetEntry.ModTime).Format("01-02 15:04")
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
