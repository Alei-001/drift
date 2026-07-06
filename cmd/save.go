package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/porcelain"
	"github.com/your-org/drift/internal/storage"
)

var saveMessage string
var saveTag string

var saveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save a snapshot of current workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		store, cfg, err := openProjectOrReport("Save", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		message := saveMessage
		if message == "" {
			statusFailed("Save", "-m <message> is required.", "use 'drift save -m \"your message\"' to describe this snapshot.")
			return ErrSilent
		}

		author := cfg.User.Name
		if author == "" {
			author = "drift"
		}

		var tags []string
		if saveTag != "" {
			tags = []string{saveTag}
		}
		snapshot, err := porcelain.CreateSnapshot(ctx, store, cwd, message, author, tags, &cfg.Core)
		if err != nil {
			if errors.Is(err, porcelain.ErrNothingToSave) {
				statusFailed("Save", "nothing to save.", "modify some files first to create a meaningful checkpoint.")
				return ErrSilent
			}
			return err
		}

		// Compute added/modified/deleted by comparing with the previous snapshot.
		// The snapshot is already saved at this point; computeChanges only
		// produces the diff display, so a failure here is downgraded to a
		// warning rather than aborting the command.
		add, mod, del, err := computeChanges(ctx, store, snapshot)
		changesOK := true
		if err != nil {
			slog.Warn("compute changes failed", "error", err)
			add, mod, del = nil, nil, nil
			changesOK = false
		}

		sid := snapshot.ShortID()
		msgLine := snapshot.Message
		if saveTag != "" {
			if err := porcelain.SaveTag(ctx, store, cwd, saveTag, snapshot.ID.Hash); err != nil {
				// The snapshot was already saved successfully; only the tag failed.
				// Report it as a warning rather than a hard failure so the user
				// knows the snapshot exists and can retry tagging it.
				statusOK("Saved [%s]", sid)
				fmt.Println(msgLine)
				fmt.Fprintf(os.Stderr, "  warning: tag '%s' failed: %v\n", saveTag, err)
				return ErrSilent
			}
			msgLine += "  [" + saveTag + "]"
		}
		statusOK("Saved [%s]", sid)
		fmt.Println(msgLine)

		if changesOK {
			// Print file list with sizes
			printFileListWithSize(add, mod, del)

			// Summary
			total := len(add) + len(mod) + len(del)
			if total > 0 {
				summaryLine(total, len(add), len(mod), len(del))
			}
		}
		return nil
	},
}

func computeChanges(ctx context.Context, store storage.Storer, snapshot *core.Snapshot) (added []core.FileEntry, modified []core.FileEntry, deleted []string, err error) {
	currFiles := make(map[string]core.FileEntry)
	for _, f := range snapshot.Files {
		currFiles[f.Path] = f
	}

	// Get previous snapshot files
	var prevFiles map[string]core.FileEntry
	if snapshot.PrevID != nil {
		prevSnap, err := store.GetSnapshot(ctx, *snapshot.PrevID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read previous snapshot: %w", err)
		}
		prevFiles = make(map[string]core.FileEntry)
		for _, f := range prevSnap.Files {
			prevFiles[f.Path] = f
		}
	}

	// Find added and modified
	for _, f := range snapshot.Files {
		if prevFiles == nil {
			added = append(added, f)
			continue
		}
		if prev, ok := prevFiles[f.Path]; !ok {
			added = append(added, f)
		} else if prev.Size != f.Size || !slices.Equal(prev.Chunks, f.Chunks) {
			modified = append(modified, f)
		}
	}

	// Find deleted
	if prevFiles != nil {
		for p := range prevFiles {
			if _, ok := currFiles[p]; !ok {
				deleted = append(deleted, p)
			}
		}
	}

	return added, modified, deleted, nil
}

func init() {
	saveCmd.Flags().StringVarP(&saveMessage, "message", "m", "", "snapshot message (required)")
	saveCmd.Flags().StringVar(&saveTag, "tag", "", "tag name for this snapshot")
	rootCmd.AddCommand(saveCmd)
}
