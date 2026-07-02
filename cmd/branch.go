package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
)

var branchDelete bool
var branchMove bool

var branchCmd = &cobra.Command{
	Use:   "branch [<name>]",
	Short: "Create, list, delete, or rename branches",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) > 2 {
			return fmt.Errorf("too many arguments")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Branch", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		if branchDelete && branchMove {
			statusFailed("Branch", "cannot use -d and -m together.", "")
			return ErrSilent
		}

		if branchDelete {
			if len(args) == 0 {
				statusFailed("Branch", "branch name required with -d.", "use 'drift branch <name>' to delete a branch.")
				return ErrSilent
			}
			if len(args) > 1 {
				statusFailed("Branch", "branch delete accepts at most one argument.", "")
				return ErrSilent
			}
			name := args[0]
			err := porcelain.DeleteBranch(ctx, store, cwd, name)
			if err != nil {
				var hint string
			switch {
			case errors.Is(err, porcelain.ErrBranchNotFound):
				hint = "use 'drift branch' to list existing branches."
				statusFailed("Branch", fmt.Sprintf("branch '%s' not found.", name), hint)
			case errors.Is(err, porcelain.ErrCannotDeleteCurrentBranch):
				hint = "switch to another branch first with 'drift switch'."
				statusFailed("Branch", err.Error(), hint)
			case errors.Is(err, porcelain.ErrCannotDeleteMain):
				hint = "'main' is the default branch and cannot be removed."
				statusFailed("Branch", err.Error(), hint)
				default:
					statusFailed("Branch", err.Error(), "")
				}
				return ErrSilent
			}
			statusOK("Branch deleted")
			fmt.Printf("'%s' has been removed.\n", name)
			return nil
		}

		if branchMove {
			if len(args) == 0 {
				statusFailed("Branch", "new branch name required with -m.", "use 'drift branch -m <new-name>' to rename the current branch.")
				return ErrSilent
			}
			var oldName, newName string
			if len(args) == 1 {
				// Rename the current branch.
				headRef, err := store.GetRef(ctx, "HEAD")
				if err != nil {
					return fmt.Errorf("read HEAD: %w", err)
				}
				if headRef.SymRef == "" {
					statusFailed("Branch", "HEAD is detached; specify both old and new branch names.", "use 'drift branch -m <old> <new>' instead.")
					return ErrSilent
				}
				oldName = strings.TrimPrefix(headRef.SymRef, "heads/")
				newName = args[0]
			} else {
				oldName = args[0]
				newName = args[1]
			}
			err := porcelain.RenameBranch(ctx, store, cwd, oldName, newName)
			if err != nil {
				var hint string
				switch {
				case errors.Is(err, porcelain.ErrBranchNotFound):
					hint = "use 'drift branch' to list existing branches."
					statusFailed("Branch", fmt.Sprintf("branch '%s' not found.", oldName), hint)
				case errors.Is(err, porcelain.ErrBranchAlreadyExists):
					hint = "use 'drift branch' to list existing branches."
					statusFailed("Branch", err.Error(), hint)
			case errors.Is(err, porcelain.ErrCannotRenameMain):
				hint = "'main' is the default branch and cannot be renamed."
				statusFailed("Branch", err.Error(), hint)
				default:
					statusFailed("Branch", err.Error(), "")
				}
				return ErrSilent
			}
			statusOK("Branch renamed")
			fmt.Printf("'%s' has been renamed to '%s'.\n", oldName, newName)
			return nil
		}

		if len(args) == 0 {
			branches, current, err := porcelain.ListBranches(ctx, store)
			if err != nil {
				return err
			}
			// Sort: current branch first, then others alphabetically.
			sort.Slice(branches, func(i, j int) bool {
				iName := branches[i].Name
				jName := branches[j].Name
				if idx := strings.LastIndex(iName, "/"); idx >= 0 {
					iName = iName[idx+1:]
				}
				if idx := strings.LastIndex(jName, "/"); idx >= 0 {
					jName = jName[idx+1:]
				}
				if iName == current {
					return true
				}
				if jName == current {
					return false
				}
				return iName < jName
			})
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

		if len(args) > 1 {
			statusFailed("Branch", "too many arguments for branch creation.", "use 'drift branch <name>' to create a branch.")
			return ErrSilent
		}
		name := args[0]
		err = porcelain.CreateBranch(ctx, store, cwd, name)
		if err != nil {
			if errors.Is(err, porcelain.ErrBranchAlreadyExists) {
				statusFailed("Branch", fmt.Sprintf("'%s' already exists.", name), "use 'drift switch "+name+"' to switch to it.")
				return ErrSilent
			}
			statusFailed("Branch", err.Error(), "")
			return ErrSilent
		}
		sid := "no commits yet"
		headRef, err := store.GetRef(ctx, "HEAD")
		if err == nil && headRef != nil && !headRef.Target.IsZero() {
			snap, snapErr := store.GetSnapshot(ctx, core.SnapshotID{Hash: headRef.Target})
			if snapErr == nil && snap != nil {
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
	branchCmd.Flags().BoolVarP(&branchMove, "move", "m", false, "rename a branch")
	rootCmd.AddCommand(branchCmd)
}
