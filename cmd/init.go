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
		err := porcelain.InitProject(target)
		if err != nil {
			return err
		}
		fmt.Println("Initialized empty drift repository in " + filepath.Join(target, ".drift"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
