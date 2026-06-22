package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var branchCmd = &cobra.Command{
	Use:   "branch [name]",
	Short: "List or create branches",
	Long: `List all branches, or create a new branch from the current version.
Without arguments, lists all branches.
With a name, creates a new branch pointing to the current commit.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "list" {
			return listBranches()
		}
		return createBranch(args[0])
	},
}

func init() {
	rootCmd.AddCommand(branchCmd)
}

func listBranches() error {
	currentBranch, _ := sharedStore.GetRef("HEAD")
	if currentBranch == "" {
		currentBranch = "main"
	}

	refs, err := sharedStore.ListRefs()
	if err != nil {
		return fmt.Errorf("failed to list refs: %w", err)
	}

	if len(refs) == 0 {
		fmt.Println("No branches yet")
		return nil
	}

	names := make([]string, 0, len(refs))
	for name := range refs {
		if name == "HEAD" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		prefix := "  "
		if name == currentBranch {
			prefix = "* "
		}
		fmt.Printf("%s%s\n", prefix, name)
	}

	return nil
}

func createBranch(name string) error {
	// Issue 12: refuse to overwrite an existing branch.
	if existing, err := sharedStore.GetRef(name); err == nil || existing != "" {
		// GetRef returns ErrObjectNotFound for missing refs; any other result
		// means the branch already exists.
		_ = existing
		return fmt.Errorf("branch %q already exists", name)
	}

	currentBranch, _ := sharedStore.GetRef("HEAD")
	if currentBranch == "" {
		currentBranch = "main"
	}

	// Get the current branch's commit hash (empty string if no commits yet)
	commitHash, err := sharedStore.GetRef(currentBranch)
	if err != nil {
		// If the ref doesn't exist (no commits yet), use empty hash
		commitHash = ""
	}

	if err := sharedStore.SaveRef(name, commitHash); err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	fmt.Printf("Created branch: %s\n", name)
	return nil
}
