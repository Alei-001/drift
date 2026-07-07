package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/porcelain"
)

var saveMessage string
var saveTags []string

var saveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save a snapshot of current workspace",
	Long:  "Save a snapshot of the current workspace. All changes (added, modified, deleted files) are captured automatically; there is no staging area. The optional -m flag sets a message; when omitted a default 'snapshot <timestamp>' message is used. The --tag flag (repeatable) attaches one or more tag names to the new snapshot.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}

		store, cfg, err := porcelain.OpenProject(cwd)
		if err != nil {
			reportFailed("Save", "save", "not a drift repository.", "use 'drift init' to create one.")
			return ErrSilent
		}
		defer store.Close()

		// -m is optional: when omitted, synthesize a default message so
		// CreateSnapshot (which requires a non-empty message) still works.
		// The display layer prefixes "[no message]" to signal the auto message.
		noMessage := saveMessage == ""
		message := saveMessage
		if noMessage {
			message = "snapshot " + time.Now().Format("2006-01-02 15:04")
		}

		author := cfg.User.Name
		if author == "" {
			author = "drift"
		}

		// Collect tag names, skipping empties. Existence is re-checked
		// after the save (within AddTag's own lock), so a pre-check here
		// would only save an AddTag round-trip; we keep the save path
		// simple and let AddTag report duplicates as warnings.
		var tagsToCreate []string
		var tagWarn strings.Builder
		for _, t := range saveTags {
			if t == "" {
				continue
			}
			tagsToCreate = append(tagsToCreate, t)
		}

		snapshot, err := porcelain.CreateSnapshot(ctx, store, cwd, message, author, &cfg.Core)
		if err != nil {
			if errors.Is(err, porcelain.ErrNothingToSave) {
				reportFailed("Save", "save", "nothing to save.", "modify some files first to create a meaningful checkpoint.")
				return ErrSilent
			}
			return err
		}

		// Compute added/modified/deleted by comparing with the previous snapshot.
		// The snapshot is already saved at this point; SnapshotFileDiff only
		// produces the diff display, so a failure here is downgraded to a
		// warning rather than aborting the command.
		add, mod, del, err := porcelain.SnapshotFileDiff(ctx, store, snapshot)
		changesOK := true
		if err != nil {
			slog.Warn("compute changes failed", "error", err)
			add, mod, del = nil, nil, nil
			changesOK = false
		}

		// Create tag refs for each non-empty tag name. AddTag validates
		// the name, checks snapshot existence, and writes the ref under
		// the workspace lock. A tag failure does not undo the snapshot;
		// it is reported as a warning and the command exits 1 so scripts
		// can detect the partial failure.
		var successTags []string
		for _, t := range tagsToCreate {
			if err := porcelain.AddTag(ctx, store, cwd, t, snapshot.ID); err != nil {
				fmt.Fprintf(&tagWarn, "  warning: tag '%s' failed: %v\n", t, err)
			} else {
				successTags = append(successTags, t)
			}
		}

		sid := snapshot.ShortID()

		if globalJSON {
			tags := successTags
			if tags == nil {
				tags = []string{}
			}
			status := "ok"
			var jsonHint string
			if tagWarn.Len() > 0 {
				status = "warning"
				jsonHint = "some tags failed; see warnings on stderr."
			}
			if err := outputJSON(JSONEnvelope{
				Command: "save",
				Status:  status,
				Data: saveData{
					ID:      sid,
					Message: snapshot.Message,
					Tags:    tags,
					Files:   buildSaveFiles(add, mod, del),
				},
				Hint: hintPtr(jsonHint),
			}); err != nil {
				return err
			}
			if tagWarn.Len() > 0 {
				fmt.Fprint(os.Stderr, tagWarn.String())
				return ErrSilent
			}
			return nil
		}

		// Human-friendly output (suppressed in quiet mode; errors and tag
		// warnings remain visible).
		if !globalQuiet {
			msgLine := snapshot.Message
			if noMessage {
				msgLine = "[no message] " + snapshot.Message
			}
			if len(successTags) > 0 {
				formatted := make([]string, len(successTags))
				for i, t := range successTags {
					formatted[i] = "[" + t + "]"
				}
				msgLine += "  " + strings.Join(formatted, " ")
			}
			statusOK("Saved [%s]", sid)
			fmt.Println(msgLine)
			if changesOK {
				printFileListWithSize(add, mod, del)
				total := len(add) + len(mod) + len(del)
				if total > 0 {
					summaryLine(total, len(add), len(mod), len(del))
				}
			}
		}

		if tagWarn.Len() > 0 {
			fmt.Fprint(os.Stderr, tagWarn.String())
			return ErrSilent
		}
		return nil
	},
}

// saveFile is a single file entry in the JSON output of `drift save`.
type saveFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Size   int64  `json:"size"`
}

// saveData is the JSON data payload of `drift save` on success.
type saveData struct {
	ID      string     `json:"id"`
	Message string     `json:"message"`
	Tags    []string   `json:"tags"`
	Files   []saveFile `json:"files"`
}

// buildSaveFiles assembles the JSON file list from the added, modified, and
// deleted file sets produced by porcelain.SnapshotFileDiff. The returned slice
// is always non-nil so that an empty change set serializes as [] rather than
// null.
func buildSaveFiles(added, modified []core.FileEntry, deleted []string) []saveFile {
	files := []saveFile{}
	for _, f := range added {
		files = append(files, saveFile{Path: f.Path, Status: "added", Size: f.Size})
	}
	for _, f := range modified {
		files = append(files, saveFile{Path: f.Path, Status: "modified", Size: f.Size})
	}
	for _, p := range deleted {
		files = append(files, saveFile{Path: p, Status: "deleted"})
	}
	return files
}

func init() {
	saveCmd.Flags().StringVarP(&saveMessage, "message", "m", "", "snapshot message (optional; defaults to 'snapshot <timestamp>')")
	saveCmd.Flags().StringArrayVar(&saveTags, "tag", nil, "tag name for this snapshot (repeatable)")
	rootCmd.AddCommand(saveCmd)
}
