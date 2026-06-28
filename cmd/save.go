package cmd

import (
	"fmt"
	"os"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/spf13/cobra"
)

var saveMessage string
var saveTag string

var saveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save a snapshot of current workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()

		store, cfg, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		message := saveMessage
		if message == "" {
			statusFailed("Save", "-m <message> is required.", "use 'drift save -m \"your message\"' to describe this snapshot.")
			return fmt.Errorf("message required")
		}

		author := cfg.User.Name
		if author == "" {
			author = "drift"
		}

		var tags []string
		if saveTag != "" {
			tags = []string{saveTag}
		}
		snapshot, err := porcelain.CreateSnapshot(store, cwd, message, author, tags)
		if err != nil {
			if err.Error() == "nothing to save" {
				statusFailed("Save", "nothing to save.", "modify some files first, or use --allow-empty.")
				return fmt.Errorf("nothing to save")
			}
			return err
		}

		// Compute added/modified/deleted by comparing with the previous snapshot
		add, mod, del := computeChanges(store, snapshot)

		sid := snapshot.ShortID()
		msgLine := snapshot.Message
		if saveTag != "" {
			ref := &core.Reference{
				Type:   core.RefTypeTag,
				Name:   saveTag,
				Target: snapshot.ID.Hash,
			}
			if err := store.SetRef("tags/"+saveTag, ref); err != nil {
				return err
			}
			msgLine += "  [" + saveTag + "]"
		}
		statusOK("Saved [%s]", sid)
		fmt.Println(msgLine)

		// Print file list with sizes
		printFileListWithSize(add, mod, del)

		// Summary
		total := len(add) + len(mod) + len(del)
		if total > 0 {
			summaryLine(total, len(add), len(mod), len(del))
		}
		return nil
	},
}

func computeChanges(store storage.Storer, snapshot *core.Snapshot) (added []core.FileEntry, modified []core.FileEntry, deleted []string) {
	currFiles := make(map[string]core.FileEntry)
	for _, f := range snapshot.Files {
		currFiles[f.Path] = f
	}

	// Get previous snapshot files
	var prevFiles map[string]core.FileEntry
	if snapshot.PrevID != nil {
		prevSnap, err := store.GetSnapshot(*snapshot.PrevID)
		if err == nil {
			prevFiles = make(map[string]core.FileEntry)
			for _, f := range prevSnap.Files {
				prevFiles[f.Path] = f
			}
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
		} else if prev.Size != f.Size || !chunkHashesEqual(prev.Chunks, f.Chunks) {
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

	return added, modified, deleted
}

func init() {
	saveCmd.Flags().StringVarP(&saveMessage, "message", "m", "", "snapshot message (required)")
	saveCmd.Flags().StringVar(&saveTag, "tag", "", "tag name for this snapshot")
	rootCmd.AddCommand(saveCmd)
}
