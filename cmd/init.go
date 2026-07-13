package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a new drift repository",
	Long:  "Initialize a new drift repository in the given path (or the current directory when omitted). This creates a .drift/ directory holding the local object store, refs, and config. The command is idempotent: re-running it in an existing repository reports 'already a drift repository.' and exits non-zero. The global -C/--cwd option may be used to target a directory other than the current one.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Honor the global -C/--cwd option so 'drift -C <path> init' works
		// the same as 'drift init <path>'. cli-design.md documents both.
		base, err := getCwd()
		if err != nil {
			return err
		}
		var target string
		if len(args) > 0 {
			target, err = filepath.Abs(filepath.Join(base, args[0]))
			if err != nil {
				return err
			}
		} else {
			target = base
		}

		driftDir := filepath.Join(target, ".drift")
		if _, err := os.Stat(driftDir); err == nil {
			reportFailed("Init", "init", "already a drift repository.", "use 'drift status' to check current state.")
			return ErrSilent
		}

		if err := porcelain.InitProject(target); err != nil {
			return err
		}
		if globalJSON {
			return outputJSON(JSONEnvelope{
				Command: "init",
				Status:  "ok",
				Data:    initData{Path: driftDir},
			})
		}
		if !globalQuiet {
			statusOK("Initialized")
			fmt.Println(driftDir)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}

// initData is the JSON payload for a successful drift init.
type initData struct {
	Path string `json:"path"`
}
