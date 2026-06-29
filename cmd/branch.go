package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
)

var branchDelete bool

var branchCmd = &cobra.Command{
	Use:   "branch [<name>]",
	Short: "Create, list, or delete branches",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if branchDelete {
			if len(args) == 0 {
				return fmt.Errorf("branch name required with -d")
			}
			name := args[0]
			err := porcelain.DeleteBranch(store, name)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					statusFailed("Branch", fmt.Sprintf("branch '%s' not found.", name), "use 'drift branch' to list existing branches.")
					return err
				}
				if strings.Contains(err.Error(), "cannot delete the current branch") {
					statusFailed("Branch", err.Error(), "switch to another branch first with 'drift switch'.")
					return err
				}
				if strings.Contains(err.Error(), "cannot delete 'main'") {
					statusFailed("Branch", err.Error(), "'main' is the default branch and cannot be removed.")
					return err
				}
				statusFailed("Branch", err.Error(), "")
				return err
			}
			statusOK("Branch deleted")
			fmt.Printf("'%s' has been removed.\n", name)
			return nil
		}

		if len(args) == 0 {
			branches, current, err := porcelain.ListBranches(store)
			if err != nil {
				return err
			}
			fmt.Printf(">>> Branches (%d)\n", len(branches))
			for _, b := range branches {
				displayName := b.Name
				if idx := strings.LastIndex(b.Name, "/"); idx >= 0 {
					displayName = b.Name[idx+1:]
				}
				if displayName == current {
					fmt.Printf("* %s\n", displayName)
				} else {
					fmt.Printf("  %s\n", displayName)
				}
			}
			return nil
		}

		name := args[0]
		err = porcelain.CreateBranch(store, name)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				statusFailed("Branch", fmt.Sprintf("'%s' already exists.", name), "use 'drift switch "+name+"' to switch to it.")
				return err
			}
			statusFailed("Branch", err.Error(), "")
			return err
		}
		headRef, _ := store.GetRef("HEAD")
	sid := "no commits yet"
	if !headRef.Target.IsZero() {
		snap, _ := store.GetSnapshot(core.SnapshotID{Hash: headRef.Target})
		if snap != nil {
			sid = snap.ShortID()
		} else {
			sid = headRef.Target.String()
		}
	}
	statusOK("Branch created")
	fmt.Printf("'%s' at snapshot %s.\n", name, sid)
	return nil
	},
}

func init() {
	branchCmd.Flags().BoolVarP(&branchDelete, "delete", "d", false, "delete a branch")
	rootCmd.AddCommand(branchCmd)
}
