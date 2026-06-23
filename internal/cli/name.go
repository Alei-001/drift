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

var nameCmd = &cobra.Command{
	Use:   "name",
	Short: "Manage version names (aliases for commits)",
	Long:  "Assign human-readable names to versions, e.g. 'drift name v3 客户终稿'.",
}

var nameAddCmd = &cobra.Command{
	Use:   "name <version> <label>",
	Short: "Assign a name to a version",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		version := args[0]
		label := args[1]

		// Validate label: no path separators, no dots-only, reasonable length.
		if err := validateNameLabel(label); err != nil {
			return err
		}

		// Resolve the version to a commit hash.
		commit, err := resolveCommit(sharedStore, version)
		if err != nil {
			return err
		}

		// Store the name as a ref under names/<label>.
		refName := "names/" + label
		if err := sharedStore.SaveRef(refName, commit.Hash); err != nil {
			return fmt.Errorf("failed to save name: %w", err)
		}

		fmt.Printf("Named %s (commit %s) as '%s'\n", commit.ID, commit.Hash[:12], label)
		return nil
	},
}

var nameListCmd = &cobra.Command{
	Use:   "name --list",
	Short: "List all version names",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listNames(sharedStore)
	},
}

var nameDeleteCmd = &cobra.Command{
	Use:   "name --delete <label>",
	Short: "Delete a version name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		label := args[0]
		refName := "names/" + label

		// Check if the name exists.
		if _, err := sharedStore.GetRef(refName); err != nil {
			return fmt.Errorf("name '%s' not found", label)
		}

		if err := sharedStore.DeleteRef(refName); err != nil {
			return fmt.Errorf("failed to delete name: %w", err)
		}

		fmt.Printf("Deleted name '%s'\n", label)
		return nil
	},
}

func init() {
	nameCmd.AddCommand(nameAddCmd)
	nameCmd.AddCommand(nameListCmd)
	nameCmd.AddCommand(nameDeleteCmd)
	rootCmd.AddCommand(nameCmd)
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
