package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/util/fsutil"
	"github.com/your-org/drift/util/pathutil"
)

var restoreNoBackup bool

var restoreCmd = &cobra.Command{
	Use:   "restore <snapshot-id> [<file>]",
	Short: "Restore files from a snapshot",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if len(args) < 1 {
			statusFailed("Restore", "snapshot id required.", "use 'drift log' to list available snapshots.")
			return ErrSilent
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Restore", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		snapshot := resolveSnapshot(ctx, store, args[0])
		if snapshot == nil {
			statusFailed("Restore", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
			return ErrSilent
		}

		var filePath string
		if len(args) > 1 {
			filePath = args[1]
		}

		// Compute what will change relative to the current workspace,
		// before the restore actually happens.
		var add, mod []core.FileEntry
		var del []string
		if filePath == "" {
			add, mod, del, err = computeRestoreChanges(cwd, &cfg.Core, snapshot)
			if err != nil {
				return err
			}
		}

		backupID, err := porcelain.RestoreSnapshot(ctx, store, cwd, snapshot.ID, filePath, restoreNoBackup, &cfg.Core)
		if err != nil {
			statusFailed("Restore", err.Error(), "use 'drift save' first, or restore a single file.")
			return ErrSilent
		}

		// If no backup snapshot was created (workspace was clean), fall back
		// to HEAD as the undo point so the user knows how to revert.
		if backupID == "" && !restoreNoBackup {
			headRef, refErr := store.GetRef(ctx, "HEAD")
			if refErr == nil && !headRef.Target.IsZero() {
				backupID = headRef.Target.String()
			}
		}

		if filePath != "" {
			fmt.Printf(">>> Restored %s:%s [ok]\n", snapshot.ShortID(), filePath)
			fmt.Println()
			fmt.Printf("  ~  %s\n", filePath)
			fmt.Println()
			fmt.Println("  1 file: ~1")
			if backupID != "" {
				fmt.Printf("  backup: [%s]\n", backupID)
			}
		} else {
			fmt.Printf(">>> Restored to %s [ok]\n", snapshot.ShortID())
			printFileListSimple(add, mod, del)
			total := len(add) + len(mod) + len(del)
			summaryLine(total, len(add), len(mod), len(del))
			if backupID != "" {
				fmt.Printf("  backup: [%s]\n", backupID)
			}
		}
		return nil
	},
}

// computeRestoreChanges compares the current workspace files against the
// target snapshot and returns what would change if the snapshot were restored:
// files in the snapshot but not in the workspace (added), files in both but
// with different content (modified), and files in the workspace but not in
// the snapshot (deleted). Modification is detected by comparing content
// hashes (BLAKE3) rather than modtime, since tools like "cp -p" preserve
// modtime while changing content.
func computeRestoreChanges(workDir string, cfg *core.CoreConfig, snapshot *core.Snapshot) (added []core.FileEntry, modified []core.FileEntry, deleted []string, err error) {
	type fileInfo struct {
		size int64
		path string
	}
	workspaceFiles := make(map[string]fileInfo)
	walkErr := fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel, relErr := pathutil.Rel(workDir, path)
		if relErr != nil {
			return nil
		}
		workspaceFiles[rel] = fileInfo{size: info.Size(), path: path}
		return nil
	})
	if walkErr != nil {
		return nil, nil, nil, walkErr
	}

	snapFiles := make(map[string]bool)
	for _, f := range snapshot.Files {
		snapFiles[f.Path] = true
		if ws, ok := workspaceFiles[f.Path]; !ok {
			added = append(added, f)
		} else if ws.size != f.Size {
			modified = append(modified, f)
		} else {
			// Same size: compare content hash to detect changes that
			// preserve size (e.g. "cp -p" preserves modtime too).
			workHash, hashErr := porcelain.ComputeFileHash(ws.path, cfg)
			if hashErr != nil || workHash != f.Hash {
				modified = append(modified, f)
			}
		}
	}

	for path := range workspaceFiles {
		if !snapFiles[path] {
			deleted = append(deleted, path)
		}
	}

	sort.Slice(added, func(i, j int) bool { return added[i].Path < added[j].Path })
	sort.Slice(modified, func(i, j int) bool { return modified[i].Path < modified[j].Path })
	sort.Strings(deleted)

	return added, modified, deleted, nil
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreNoBackup, "no-backup", false, "skip automatic backup before restore")
	rootCmd.AddCommand(restoreCmd)
}
