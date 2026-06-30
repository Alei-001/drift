package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/your-org/drift/porcelain"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a new drift repository",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		target := ""
		if len(args) > 0 {
			cwd, _ := os.Getwd()
			var err error
			target, err = filepath.Abs(filepath.Join(cwd, args[0]))
			if err != nil {
				return err
			}
		} else {
			target, _ = os.Getwd()
		}

		driftDir := filepath.Join(target, ".drift")
		if _, err := os.Stat(driftDir); err == nil {
			statusFailed("Init", "already a drift repository.", "use 'drift status' to check current state.")
			return nil
		}

		err := porcelain.InitProject(target)
		if err != nil {
			return err
		}
		statusOK("Initialized")
		fmt.Println(driftDir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
