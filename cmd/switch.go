package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/porcelain"
)

var switchCreate bool

var switchCmd = &cobra.Command{
	Use:   "switch <name>",
	Short: "Switch to a branch",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, cfg, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		name := args[0]
		author := cfg.User.Name
		autosaveID, fromBranch, diffCount, err := porcelain.SwitchBranch(store, cwd, name, switchCreate, author)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				statusFailed("Switch", fmt.Sprintf("branch '%s' not found.", name), "use 'drift branch' to list existing branches.")
				return err
			}
			if strings.Contains(err.Error(), "already exists") {
				statusFailed("Switch", fmt.Sprintf("branch '%s' already exists.", name), "use 'drift switch "+name+"' to switch to it.")
				return err
			}
			statusFailed("Switch", err.Error(), "")
			return err
		}

		statusOK("Switched to '%s'", name)
		fmt.Println()
		if fromBranch != "" {
			fmt.Printf("  %d files differ from %s.\n", diffCount, fromBranch)
		}
		if autosaveID != "" {
			fmt.Printf("  autosave: [%s]\n", autosaveID)
		}
		return nil
	},
}

func init() {
	switchCmd.Flags().BoolVarP(&switchCreate, "create", "c", false, "create and switch to new branch")
	rootCmd.AddCommand(switchCmd)
}
