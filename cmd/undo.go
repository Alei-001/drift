package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/porcelain"
)

// undoData is the JSON data payload of `drift undo` on success.
type undoData struct {
	RemovedID      string `json:"removed_id"`
	RemovedMessage string `json:"removed_message"`
	NewHeadID      string `json:"new_head_id"`
	NewHeadMessage string `json:"new_head_message"`
}

// undoCmd implements `drift undo`: reverts the last save by moving HEAD back
// to the previous snapshot. The undone snapshot becomes unreachable (gc will
// reclaim it). Refuses to run when the workspace has uncommitted changes.
var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Undo the last save",
	Long:  "Undo the last save. HEAD moves back to the previous snapshot; the undone snapshot becomes unreachable and will be removed by 'drift gc'.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}

		store, cfg, err := porcelain.OpenProject(cwd)
		if err != nil {
			reportFailed("Undo", "undo", "not a drift repository.", "use 'drift init' to create one.")
			return ErrSilent
		}
		defer store.Close()

		// Capture the snapshot being undone (current HEAD) before the
		// operation so we can report it to the user.
		removed := resolveSnapshot(ctx, store, "@head")

		if err := porcelain.UndoLastSave(ctx, store, cwd, &cfg.Core); err != nil {
			if errors.Is(err, porcelain.ErrCannotUndo) {
				reportFailed("Undo", "undo", "no snapshot to undo.", "HEAD is already at the initial snapshot.")
				return ErrSilent
			}
			if errors.Is(err, porcelain.ErrUncommittedChanges) {
				reportFailed("Undo", "undo", "uncommitted changes would be lost.", "use 'drift save' or 'drift restore' first.")
				return ErrSilent
			}
			return err
		}

		newHead := resolveSnapshot(ctx, store, "@head")

		hint := "the undone snapshot is now unreachable. It will be removed by 'drift gc'."

		if globalJSON {
			data := undoData{}
			if removed != nil {
				data.RemovedID = removed.ShortID()
				data.RemovedMessage = removed.Message
			}
			if newHead != nil {
				data.NewHeadID = newHead.ShortID()
				data.NewHeadMessage = newHead.Message
			}
			if err := outputJSON(JSONEnvelope{
				Command: "undo",
				Status:  "ok",
				Data:    data,
				Hint:    hintPtr(hint),
			}); err != nil {
				return err
			}
			return nil
		}

		// Human-friendly output (suppressed in quiet mode).
		if globalQuiet {
			return nil
		}

		statusOK("Undone")
		if removed != nil {
			fmt.Printf("Removed snapshot %s (%q).\n", removed.ShortID(), removed.Message)
		}
		if newHead != nil {
			fmt.Printf("HEAD now at %s (%q).\n", newHead.ShortID(), newHead.Message)
		}
		fmt.Println()
		fmt.Println("  hint: " + hint)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(undoCmd)
}
