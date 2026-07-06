package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/porcelain"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a new drift repository",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Honor the global -C/--cwd option so 'drift -C <path> init' works
		// the same as 'drift init <path>'. cli-design.md documents both.
		base, err := getCwd(cmd)
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
			statusFailed("Init", "already a drift repository.", "use 'drift status' to check current state.")
			return ErrSilent
		}

		if err := porcelain.InitProject(target); err != nil {
			return err
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
