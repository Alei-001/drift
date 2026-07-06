package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/porcelain"
)

var switchCreate bool
var switchNoAutosave bool

// switchCmd switches to an existing branch, or with -c, creates and switches.
// By default an [auto] snapshot is created before switching; --no-autosave
// skips this step but requires a clean workspace.
var switchCmd = &cobra.Command{
	Use:   "switch <name>",
	Short: "Switch to a branch",
	Long:  "Switch to an existing branch. With -c, create the branch first. With --no-autosave, skip the pre-switch auto-save (requires a clean workspace).",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd(cmd)
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Switch", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		name := args[0]
		author := cfg.User.Name
		autosaveID, fromBranch, diffCount, err := porcelain.SwitchBranch(ctx, store, cwd, name, switchCreate, switchNoAutosave, author, &cfg.Core)
		if err != nil {
			if errors.Is(err, porcelain.ErrUncommittedChanges) {
				reportFailed("Switch", "switch", "--no-autosave requires a clean working tree.", "use 'drift save' first, or drop --no-autosave to auto-save.")
				return ErrSilent
			}
			if errors.Is(err, porcelain.ErrBranchNotFound) {
				reportFailed("Switch", "switch", fmt.Sprintf("branch '%s' not found.", name), "use 'drift branch list' to list existing branches.")
				return ErrSilent
			}
			if errors.Is(err, porcelain.ErrBranchAlreadyExists) {
				reportFailed("Switch", "switch", fmt.Sprintf("branch '%s' already exists.", name), "use 'drift switch "+name+"' to switch to it.")
				return ErrSilent
			}
			reportFailed("Switch", "switch", err.Error(), "")
			return ErrSilent
		}

		if globalJSON {
			if err := outputJSON(JSONEnvelope{
				Command: "switch",
				Status:  "ok",
				Data: switchData{
					Branch:     name,
					FromBranch: fromBranch,
					DiffCount:  diffCount,
					Autosave:   autosaveID,
				},
			}); err != nil {
				return err
			}
			return nil
		}

		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}

		statusOK("Switched to '%s'", name)
		fmt.Println()
		if fromBranch != "" {
			fmt.Printf("  %d %s differ from %s.\n", diffCount, pluralFile(diffCount), fromBranch)
		}
		if autosaveID != "" {
			fmt.Printf("  autosave: [%s]\n", autosaveID)
		}
		return nil
	},
}

func init() {
	switchCmd.Flags().BoolVarP(&switchCreate, "create", "c", false, "create and switch to new branch")
	switchCmd.Flags().BoolVar(&switchNoAutosave, "no-autosave", false, "skip auto-save before switch (requires clean workspace)")
	rootCmd.AddCommand(switchCmd)
}

// switchData is the JSON data payload of `drift switch` on success.
// FromBranch and Autosave are omitted when empty.
type switchData struct {
	Branch     string `json:"branch"`
	FromBranch string `json:"from_branch,omitempty"`
	DiffCount  int    `json:"diff_count"`
	Autosave   string `json:"autosave,omitempty"`
}
