package cli

import (
	"fmt"
	"sort"

	"github.com/drift/drift/internal/repo"
	"github.com/spf13/cobra"
)

// Version names (aliases) are stored as refs under the "names/" namespace.
// A name maps a human-readable label (e.g. "客户终稿") to a commit hash,
// allowing users to reference versions by meaningful names instead of v1/v2.
//
// Storage: .drift/refs/names/<label>.ref → commit hash
//
// This is deliberately simpler than Git tags:
//   - No annotated tag objects (just a ref pointing to a commit)
//   - No GPG signing
//   - Names can contain any filename-safe characters
//
// Usage:
//   drift name <version> <label>   — assign a name
//   drift name --list              — list all names
//   drift name --delete <label>    — delete a name

var nameCmd = &cobra.Command{
	Use:   "name [<version> <label>]",
	Short: "Manage version names (aliases for commits)",
	Long: `Assign human-readable names to versions, e.g. 'drift name v3 客户终稿'.

Usage:
  drift name <version> <label>   Assign a name to a version
  drift name --list              List all version names
  drift name --delete <label>    Delete a version name`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		listFlag, _ := cmd.Flags().GetBool("list")
		deleteFlag, _ := cmd.Flags().GetString("delete")

		if listFlag {
			return listNames()
		}

		if deleteFlag != "" {
			return deleteName(deleteFlag)
		}

		// Default action: assign a name.
		if len(args) < 2 {
			return fmt.Errorf("usage: drift name <version> <label>")
		}
		return addName(args[0], args[1])
	},
}

func init() {
	nameCmd.Flags().Bool("list", false, "List all version names")
	nameCmd.Flags().String("delete", "", "Delete a version name by label")
	rootCmd.AddCommand(nameCmd)
}

// addName assigns a human-readable label to a version.
func addName(version, label string) error {
	if err := repo.ValidateNameLabel(label); err != nil {
		return err
	}

	commit, err := sharedRepo.ResolveCommit(version)
	if err != nil {
		return err
	}

	if err := sharedRepo.AddName(version, label); err != nil {
		return err
	}

	fmt.Printf("Named %s (commit %s) as '%s'\n", commit.ID, commit.Hash[:12], label)
	return nil
}

// deleteName removes a version name.
func deleteName(label string) error {
	if err := sharedRepo.DeleteName(label); err != nil {
		return err
	}
	fmt.Printf("Deleted name '%s'\n", label)
	return nil
}

// listNames prints all version names with their corresponding commits.
func listNames() error {
	entries, err := sharedRepo.ListNames()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No version names defined")
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Label < entries[j].Label
	})

	fmt.Printf("%-20s  %-12s  %s\n", "NAME", "COMMIT", "MESSAGE")
	for _, e := range entries {
		commit, err := sharedRepo.Store.GetCommit(e.Hash)
		msg := ""
		id := e.Hash[:12]
		if err == nil {
			msg = commit.Message
			id = commit.ID
		}
		// Truncate long messages for display.
		if len(msg) > 40 {
			msg = msg[:37] + "..."
		}
		fmt.Printf("%-20s  %-12s  %s\n", e.Label, id, msg)
	}

	return nil
}
