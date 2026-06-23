package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drift/drift/internal/storage"
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
			return listNames(sharedStore)
		}

		if deleteFlag != "" {
			return deleteName(sharedStore, deleteFlag)
		}

		// Default action: assign a name.
		if len(args) < 2 {
			return fmt.Errorf("usage: drift name <version> <label>")
		}
		return addName(sharedStore, args[0], args[1])
	},
}

func init() {
	nameCmd.Flags().Bool("list", false, "List all version names")
	nameCmd.Flags().String("delete", "", "Delete a version name by label")
	rootCmd.AddCommand(nameCmd)
}

// addName assigns a human-readable label to a version.
func addName(store *storage.Store, version, label string) error {
	if err := validateNameLabel(label); err != nil {
		return err
	}

	commit, err := resolveCommit(store, version)
	if err != nil {
		return err
	}

	refName := "names/" + label
	if err := store.SaveRef(refName, commit.Hash); err != nil {
		return fmt.Errorf("failed to save name: %w", err)
	}

	fmt.Printf("Named %s (commit %s) as '%s'\n", commit.ID, commit.Hash[:12], label)
	return nil
}

// deleteName removes a version name.
func deleteName(store *storage.Store, label string) error {
	refName := "names/" + label
	if _, err := store.GetRef(refName); err != nil {
		return fmt.Errorf("name '%s' not found", label)
	}
	if err := store.DeleteRef(refName); err != nil {
		return fmt.Errorf("failed to delete name: %w", err)
	}
	fmt.Printf("Deleted name '%s'\n", label)
	return nil
}

// validateNameLabel checks that a name label is safe to use as a filename.
func validateNameLabel(label string) error {
	if label == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if len(label) > 100 {
		return fmt.Errorf("name too long (max 100 characters)")
	}
	if strings.ContainsAny(label, `/\`) {
		return fmt.Errorf("name cannot contain path separators")
	}
	if label == "." || label == ".." {
		return fmt.Errorf("name cannot be '.' or '..'")
	}
	return nil
}

// listNames prints all version names with their corresponding commits.
func listNames(store *storage.Store) error {
	refs, err := store.ListRefs()
	if err != nil {
		return err
	}

	type nameEntry struct {
		label string
		hash  string
	}

	var entries []nameEntry
	for refName, hash := range refs {
		if strings.HasPrefix(refName, "names/") {
			label := strings.TrimPrefix(refName, "names/")
			entries = append(entries, nameEntry{label: label, hash: hash})
		}
	}

	if len(entries) == 0 {
		fmt.Println("No version names defined")
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].label < entries[j].label
	})

	fmt.Printf("%-20s  %-12s  %s\n", "NAME", "COMMIT", "MESSAGE")
	for _, e := range entries {
		commit, err := store.GetCommit(e.hash)
		msg := ""
		id := e.hash[:12]
		if err == nil {
			msg = commit.Message
			id = commit.ID
		}
		// Truncate long messages for display.
		if len(msg) > 40 {
			msg = msg[:37] + "..."
		}
		fmt.Printf("%-20s  %-12s  %s\n", e.label, id, msg)
	}

	return nil
}

// resolveName checks if a version string is a name alias and returns the
// commit hash if so. Returns empty string if not a name.
func resolveName(store *storage.Store, version string) string {
	refName := "names/" + version
	hash, err := store.GetRef(refName)
	if err != nil || hash == "" {
		return ""
	}
	return hash
}

// nameRefPath returns the filesystem path for a name ref file.
// Used for documentation; actual path construction is in storage layer.
func nameRefPath(store *storage.Store, label string) string {
	return filepath.Join(store.DriftDir(), "refs", "names", label+".ref")
}
