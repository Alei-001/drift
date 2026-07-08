package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
)

var restoreNoBackup bool

var restoreCmd = &cobra.Command{
	Use:   "restore <snapshot-id> [<file>]",
	Short: "Restore files from a snapshot",
	Long:  "Restore the project (or a single file) to the state of a specified snapshot. A backup is created automatically before a full restore so the operation stays reversible; --no-backup is only allowed for single-file restore.",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if len(args) < 1 {
			reportFailed("Restore", "restore", "snapshot id required.", "use 'drift log' to list available snapshots.")
			return ErrSilent
		}

		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Restore", "restore", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		snapshot := resolveSnapshot(ctx, store, args[0])
		if snapshot == nil {
			reportFailed("Restore", "restore", fmt.Sprintf("snapshot '%s' not found.", args[0]), "use 'drift log' to list available snapshots.")
			return ErrSilent
		}

		var filePath string
		if len(args) > 1 {
			filePath = args[1]
		}

		// --no-backup is only allowed for single-file restore; full
		// restore always creates a backup so the operation stays
		// reversible ("恢复永远可撤销").
		if restoreNoBackup && filePath == "" {
			reportFailed("Restore", "restore",
				"--no-backup is only allowed for single-file restore.",
				"full restore always creates a backup for safety.")
			return ErrSilent
		}

		// Compute what will change relative to the current workspace,
		// before the restore actually happens.
		var add, mod []core.FileEntry
		var del []string
		if filePath == "" {
			add, mod, del, err = porcelain.ComputeRestoreChanges(ctx, cwd, &cfg.Core, snapshot)
			if err != nil {
				return err
			}
		}

		// Capture the pre-restore HEAD so we can report it as the undo point
	// when no backup snapshot was created (clean workspace). This must be
	// read BEFORE RestoreSnapshot, which moves HEAD to the restore target.
	var prevHeadID string
	if prevHead, refErr := store.GetRef(ctx, "HEAD"); refErr == nil && !prevHead.Target.IsZero() {
		prevHeadID = prevHead.Target.String()
	}

	backupID, err := porcelain.RestoreSnapshot(ctx, store, cwd, snapshot.ID, filePath, restoreNoBackup, &cfg.Core)
	if err != nil {
		// Tailor the hint to the failure: ErrFileNotFound means the file
		// simply doesn't exist in that snapshot (the user should list
		// files with `drift show`), whereas other errors (e.g. write
		// failures) suggest saving first.
		hint := "use 'drift save' first, or restore a single file."
		if errors.Is(err, porcelain.ErrFileNotFound) {
			hint = fmt.Sprintf("use 'drift show %s' to list files in this snapshot.", args[0])
		}
		reportFailed("Restore", "restore", err.Error(), hint)
		return ErrSilent
	}

	// If no backup snapshot was created (workspace was clean), fall back
	// to the pre-restore HEAD as the undo point so the user knows how to
	// revert. Using prevHeadID (captured above) instead of re-reading HEAD
	// is essential: RestoreSnapshot already moved HEAD to the restore target.
	if backupID == "" && !restoreNoBackup {
		backupID = prevHeadID
	}

	if globalJSON {
		data := restoreData{
			Version: args[0],
			Mode:    "full",
			Backup:  backupID,
		}
		if filePath != "" {
			data.Mode = "file"
			data.File = filePath
		} else {
			data.Added = toRestoreFiles(add)
			data.Modified = toRestoreFiles(mod)
			data.Deleted = del
			data.Summary = &restoreSummary{
				Total:    len(add) + len(mod) + len(del),
				Added:    len(add),
				Modified: len(mod),
				Deleted:  len(del),
			}
		}
		if err := outputJSON(JSONEnvelope{
			Command: "restore",
			Status:  "ok",
			Data:    data,
		}); err != nil {
			return err
		}
		return nil
	}

	// Quiet mode: success produces no output (exit code is authoritative).
	if globalQuiet {
		return nil
	}

	// Use args[0] (the user-supplied version reference) in the
	// status line so the output echoes what the user typed, e.g.
	// ">>> Restored to id:12ab [ok]".
	if filePath != "" {
			fmt.Printf(">>> Restored %s:%s [ok]\n", args[0], filePath)
			fmt.Println()
			fmt.Printf("  ~  %s\n", filePath)
			fmt.Println()
			fmt.Println("  1 file: ~1")
			if backupID != "" {
				fmt.Printf("  backup: [%s]\n", backupID)
			}
		} else {
			fmt.Printf(">>> Restored to %s [ok]\n", args[0])
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

func init() {
	restoreCmd.Flags().BoolVar(&restoreNoBackup, "no-backup", false, "skip automatic backup before restore")
	rootCmd.AddCommand(restoreCmd)
}

// restoreFile is a single file entry in the JSON output of `drift restore`.
type restoreFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// restoreSummary is the per-category change tally for a full restore.
type restoreSummary struct {
	Total    int `json:"total"`
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

// restoreData is the JSON data payload of `drift restore` on success. The
// schema differs by mode: "full" includes added/modified/deleted/summary,
// "file" includes file. Backup is omitted when no backup was created.
type restoreData struct {
	Version  string          `json:"version"`
	Mode     string          `json:"mode"`
	File     string          `json:"file,omitempty"`
	Added    []restoreFile   `json:"added,omitempty"`
	Modified []restoreFile   `json:"modified,omitempty"`
	Deleted  []string        `json:"deleted,omitempty"`
	Summary  *restoreSummary `json:"summary,omitempty"`
	Backup   string          `json:"backup,omitempty"`
}

// toRestoreFiles converts a slice of FileEntry to restoreFile entries for
// JSON serialization. The returned slice is always non-nil so that a
// non-empty change set serializes as a JSON array.
func toRestoreFiles(entries []core.FileEntry) []restoreFile {
	out := make([]restoreFile, 0, len(entries))
	for _, f := range entries {
		out = append(out, restoreFile{Path: f.Path, Size: f.Size})
	}
	return out
}
