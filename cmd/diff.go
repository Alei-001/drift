package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/spf13/cobra"
)

var diffStatOnly bool

var diffCmd = &cobra.Command{
	Use:   "diff [--stat] [<base>] [<target>] [-- <file>]",
	Short: "Show changes between snapshots or workspace",
	Long: "Diff shows changes between snapshots or the workspace.\n" +
		"\n" +
		"Without arguments: workspace vs HEAD.\n" +
		"One snapshot argument: workspace vs that snapshot.\n" +
		"Two snapshot arguments: between two snapshots.\n" +
		"-- <file>: limit to a single file (text diff or binary metadata).\n" +
		"--stat: file-level summary only (no line-level diff).",
	Args: cobra.MaximumNArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Diff", "diff", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		dashIdx := cmd.ArgsLenAtDash()
		var snapArgs, fileArgs []string
		if dashIdx >= 0 {
			snapArgs = args[:dashIdx]
			fileArgs = args[dashIdx:]
		} else {
			snapArgs = args
		}

		if len(snapArgs) > 2 {
			reportFailed("Diff", "diff", "too many snapshot arguments (max 2).",
				"use -- <file> to limit diff to a single file.", nil)
			return ErrSilent
		}
		if len(fileArgs) > 1 {
			reportFailed("Diff", "diff", "only one file may be specified after --.", "", nil)
			return ErrSilent
		}

		file := ""
		if len(fileArgs) == 1 {
			file = fileArgs[0]
		}

		switch len(snapArgs) {
		case 0:
			headSnap := porcelain.ResolveHeadSnapshot(ctx, store)
			if headSnap == nil {
			reportFailed("Diff", "diff", "no snapshot to compare against.",
				"use 'drift save -m \"message\"' to create one first.", nil)
				return ErrSilent
			}
			return runDiffWorkspaceVs(ctx, store, cwd, &cfg.Core, headSnap, "head", file)
		case 1:
			snap1 := resolveSnapshot(ctx, store, snapArgs[0])
			if snap1 == nil {
				reportFailed("Diff", "diff", fmt.Sprintf("snapshot '%s' not found.", snapArgs[0]),
					"use 'drift log' to list available snapshots.", nil)
				return ErrSilent
			}
			return runDiffWorkspaceVs(ctx, store, cwd, &cfg.Core, snap1, snapArgs[0], file)
		default:
			snap1 := resolveSnapshot(ctx, store, snapArgs[0])
			if snap1 == nil {
				reportFailed("Diff", "diff", fmt.Sprintf("snapshot '%s' not found.", snapArgs[0]),
					"use 'drift log' to list available snapshots.", nil)
				return ErrSilent
			}
			snap2 := resolveSnapshot(ctx, store, snapArgs[1])
			if snap2 == nil {
				reportFailed("Diff", "diff", fmt.Sprintf("snapshot '%s' not found.", snapArgs[1]),
					"use 'drift log' to list available snapshots.", nil)
				return ErrSilent
			}
			return runDiffSnapshots(ctx, store, cwd, &cfg.Core, snap1, snap2, snapArgs[0], snapArgs[1], file)
		}
	},
}

// runDiffWorkspaceVs handles workspace-vs-snapshot diff. The status line is
// printed here (using the user-supplied label) and porcelain prints the body.
func runDiffWorkspaceVs(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot, snapLabel, file string) error {
	if globalJSON {
		if file != "" {
			return diffWorkspaceFileJSON(ctx, store, cwd, snap, snapLabel, file)
		}
		if diffStatOnly {
			return diffStatWorkspaceJSON(ctx, store, cwd, cfg, snap, snapLabel)
		}
		return diffWorkspaceJSON(ctx, store, cwd, cfg, snap, snapLabel)
	}
	// Quiet mode: suppress all diff output (status line + body).
	if globalQuiet {
		return nil
	}
	if file != "" {
		fmt.Printf(">>> Diff %s -> workspace %s\n", snapLabel, file)
		result, err := porcelain.DiffWorkspaceFileVsSnapshot(ctx, store, cwd, snap, file)
		if err != nil {
			return err
		}
		printContentDiff(result)
		return nil
	}
	if diffStatOnly {
		fmt.Printf(">>> Diff %s -> workspace (stat)\n", snapLabel)
		return diffStatWorkspace(ctx, store, cwd, cfg, snap)
	}
	fmt.Printf(">>> Diff %s -> workspace\n", snapLabel)
	result, err := porcelain.DiffWorkspaceVsSnapshot(ctx, cwd, snap, cfg)
	if err != nil {
		return err
	}
	printFileDiff(result)
	return nil
}

// runDiffSnapshots handles snapshot-vs-snapshot diff.
func runDiffSnapshots(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap1, snap2 *core.Snapshot, label1, label2, file string) error {
	if globalJSON {
		if file != "" {
			return diffFileJSON(ctx, store, cwd, snap1, snap2, label1, label2, file)
		}
		if diffStatOnly {
			return diffStatSnapshotsJSON(ctx, store, snap1, snap2, label1, label2)
		}
		return diffSnapshotsJSON(ctx, store, snap1, snap2, label1, label2)
	}
	// Quiet mode: suppress all diff output (status line + body).
	if globalQuiet {
		return nil
	}
	if file != "" {
		fmt.Printf(">>> Diff %s -> %s %s\n", label1, label2, file)
		result := porcelain.DiffFileInSnapshots(ctx, store, cwd, snap1, snap2, file)
		printContentDiff(result)
		return nil
	}
	if diffStatOnly {
		fmt.Printf(">>> Diff %s -> %s (stat)\n", label1, label2)
		return diffStatSnapshots(ctx, store, snap1, snap2)
	}
	fmt.Printf(">>> Diff %s -> %s\n", label1, label2)
	result := porcelain.DiffSnapshots(snap1, snap2)
	printFileDiff(result)
	return nil
}

// printFileDiff prints the file-level diff body: the added/modified/deleted
// file lists and a summary line. The status line is emitted by the caller.
func printFileDiff(result porcelain.FileDiffResult) {
	total := len(result.Added) + len(result.Modified) + len(result.Deleted)
	if total == 0 {
		fmt.Println()
		fmt.Println("  No changes.")
		return
	}
	fmt.Println()
	for _, p := range result.Added {
		fmt.Printf("  +  %s\n", p)
	}
	for _, p := range result.Modified {
		fmt.Printf("  ~  %s\n", p)
	}
	for _, p := range result.Deleted {
		fmt.Printf("  -  %s\n", p)
	}
	summaryLine(total, len(result.Added), len(result.Modified), len(result.Deleted))
}

// printContentDiff prints a content-level diff result to stdout/stderr.
func printContentDiff(result porcelain.ContentDiffResult) {
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
}

func init() {
	diffCmd.Flags().BoolVar(&diffStatOnly, "stat", false, "show file-level summary only")
	rootCmd.AddCommand(diffCmd)
}
