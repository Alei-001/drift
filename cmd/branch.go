package cmd

import (
	"github.com/Alei-001/drift/internal/errs"
	"github.com/Alei-001/drift/internal/branch"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// branchCmd is the parent command for branch management. It has no RunE so
// cobra displays help when invoked without a subcommand.
var branchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Manage branches (list, create, delete, rename)",
	Long:  "Manage branches. Subcommands: list, create, delete, rename.",
}

// branchListCmd lists all branches, marking the current one with '*'.
var branchListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all branches",
	Long:  "List all branches, marking the current branch with '*'.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Branch", "branch", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		branches, current, err := branch.ListBranches(ctx, store)
		if err != nil {
			return err
		}
		// ListBranches returns refs whose Name is the full ref path
		// ("heads/main"); current is the bare branch name ("main").
		// Trim the prefix so the comparison works and the output shows
		// bare branch names per cli-design.md.
		sort.Slice(branches, func(i, j int) bool {
			ni := strings.TrimPrefix(branches[i].Name, "heads/")
			nj := strings.TrimPrefix(branches[j].Name, "heads/")
			return ni < nj
		})
		if globalJSON {
			entries := make([]branchJSONEntry, 0, len(branches))
			for _, b := range branches {
				name := strings.TrimPrefix(b.Name, "heads/")
				entries = append(entries, branchJSONEntry{
					Name:    name,
					Current: name == current,
				})
			}
			return outputJSON(JSONEnvelope{
				Command: "branch",
				Status:  "ok",
				Data:    branchJSONData{Branches: entries},
			})
		}
		// Quiet mode: success produces no output (exit code is authoritative).
		if globalQuiet {
			return nil
		}
		fmt.Printf(">>> Branches (%d)\n", len(branches))
		for _, b := range branches {
			name := strings.TrimPrefix(b.Name, "heads/")
			if name == current {
				fmt.Printf("* %s\n", name)
			} else {
				fmt.Printf("  %s\n", name)
			}
		}
		return nil
	},
}

// branchCreateCmd creates a new branch pointing at the current HEAD snapshot.
// It does not switch to the new branch.
var branchCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new branch (does not switch)",
	Long:  "Create a new branch pointing at the current HEAD snapshot. Does not switch to it.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Branch", "branch", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		name := args[0]
		tipID, err := branch.CreateBranch(ctx, store, cwd, name)
		if err != nil {
			if errors.Is(err, errs.ErrBranchAlreadyExists) {
				reportFailed("Branch", "branch", fmt.Sprintf("'%s' already exists.", name), "use 'drift switch "+name+"' to switch to it.", err)
				return silentWrap(err)
			}
			reportFailed("Branch", "branch", "could not create branch.", "", err)
			return silentWrap(err)
		}
		sid := "no commits yet"
		if !tipID.Hash.IsZero() {
			sid = tipID.Hash.String()
		}
		if !globalQuiet {
			statusOK("Branch created")
			fmt.Printf("'%s' at snapshot %s.\n", name, sid)
		}
		return nil
	},
}

// branchDeleteCmd removes a branch reference. It refuses to delete the
// current branch or the protected 'main' branch.
var branchDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a branch",
	Long:  "Delete a branch reference. Refuses to delete the current branch or 'main'.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Branch", "branch", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		name := args[0]
		if err := branch.DeleteBranch(ctx, store, cwd, name); err != nil {
			switch {
			case errors.Is(err, errs.ErrBranchNotFound):
				reportFailed("Branch", "branch", fmt.Sprintf("branch '%s' not found.", name), "use 'drift branch list' to list existing branches.", err)
			case errors.Is(err, errs.ErrCannotDeleteCurrentBranch):
				reportFailed("Branch", "branch", fmt.Sprintf("cannot delete the current branch '%s'.", name), "switch to another branch first with 'drift switch'.", err)
			case errors.Is(err, errs.ErrCannotDeleteMain):
				reportFailed("Branch", "branch", fmt.Sprintf("cannot delete '%s'.", name), "'main' is the default branch and cannot be removed.", err)
			default:
				reportFailed("Branch", "branch", "could not delete branch.", "", err)
			}
			return silentWrap(err)
		}
		if !globalQuiet {
			statusOK("Branch deleted")
			fmt.Printf("'%s' has been removed.\n", name)
		}
		return nil
	},
}

// branchRenameCmd renames a branch. With one argument it renames the current
// branch; with two arguments it renames the specified branch.
var branchRenameCmd = &cobra.Command{
	Use:   "rename [<old-name>] <new-name>",
	Short: "Rename a branch",
	Long:  "Rename a branch. With one argument, renames the current branch; with two arguments, renames the specified branch.",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 && len(args) != 2 {
			return fmt.Errorf("accepts 1 or 2 args, received %d", len(args))
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := getCwd()
		if err != nil {
			return err
		}
		store, _, err := openProjectOrReport("Branch", "branch", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		var oldName, newName string
		if len(args) == 1 {
			headRef, err := store.Refs.GetRef(ctx, "HEAD")
			if err != nil {
				return fmt.Errorf("read HEAD: %w", err)
			}
			if headRef.SymRef == "" {
				reportFailed("Branch", "branch", "HEAD is detached; specify both old and new branch names.", "use 'drift branch rename <old> <new>' instead.", nil)
				return ErrSilent
			}
			oldName = strings.TrimPrefix(headRef.SymRef, "heads/")
			newName = args[0]
		} else {
			oldName = args[0]
			newName = args[1]
		}
		if err := branch.RenameBranch(ctx, store, cwd, oldName, newName); err != nil {
			switch {
			case errors.Is(err, errs.ErrBranchNotFound):
				reportFailed("Branch", "branch", fmt.Sprintf("branch '%s' not found.", oldName), "use 'drift branch list' to list existing branches.", err)
			case errors.Is(err, errs.ErrBranchAlreadyExists):
				reportFailed("Branch", "branch", fmt.Sprintf("branch '%s' already exists.", newName), "use 'drift branch list' to list existing branches.", err)
			case errors.Is(err, errs.ErrCannotRenameMain):
				reportFailed("Branch", "branch", fmt.Sprintf("cannot rename '%s'.", oldName), "'main' is the default branch and cannot be renamed.", err)
			default:
				reportFailed("Branch", "branch", "could not rename branch.", "", err)
			}
			return silentWrap(err)
		}
		if !globalQuiet {
			statusOK("Branch renamed")
			fmt.Printf("'%s' has been renamed to '%s'.\n", oldName, newName)
		}
		return nil
	},
}

func init() {
	branchCmd.AddCommand(branchListCmd)
	branchCmd.AddCommand(branchCreateCmd)
	branchCmd.AddCommand(branchDeleteCmd)
	branchCmd.AddCommand(branchRenameCmd)
	rootCmd.AddCommand(branchCmd)
}

// branchJSONEntry describes a single branch in 'drift branch list --json'.
type branchJSONEntry struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// branchJSONData is the data payload of the 'drift branch list --json' envelope.
type branchJSONData struct {
	Branches []branchJSONEntry `json:"branches"`
}
