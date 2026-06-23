package cli

import (
	"fmt"
	"strconv"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var saveCmd = &cobra.Command{
	Use:   "save [-m message]",
	Short: "Save staged changes as a new version",
	RunE: func(cmd *cobra.Command, args []string) error {
		message, _ := cmd.Flags().GetString("message")

		var idx core.Index
		if err := sharedStore.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		if len(idx.Entries) == 0 {
			return fmt.Errorf("nothing to save (use 'drift add' first)")
		}

		// Capture staged paths before the transaction clears the index.
		stagedPaths := make([]string, len(idx.Entries))
		for i, e := range idx.Entries {
			stagedPaths[i] = e.Path
		}

		builder := core.NewTreeBuilder(func(t *core.Tree) error {
			return sharedStore.PutTree(t)
		})

		tree, err := builder.BuildFromIndex(&idx)
		if err != nil {
			return fmt.Errorf("failed to build tree: %w", err)
		}

		branch, _ := sharedStore.GetRef("HEAD")
		if branch == "" {
			// HEAD was not initialized (e.g. project created before the init fix).
			// Default to "main" and persist HEAD so subsequent commands detect the branch.
			branch = "main"
			if err := sharedStore.SaveRef("HEAD", branch); err != nil {
				return fmt.Errorf("failed to initialize HEAD: %w", err)
			}
		}

		// Issue 22: derive version number from the parent chain depth, not from
		// counting all commits in the branch (which includes orphaned commits
		// after a reset). Walking the chain gives the correct depth.
		branchCommits, err := sharedStore.ListBranchCommits(branch)
		if err != nil {
			return fmt.Errorf("failed to list branch commits: %w", err)
		}
		branchCommitCount := len(branchCommits)

		parentHash := ""
		if branchCommitCount > 0 {
			parentHash = branchCommits[0].Hash
			if branchCommits[0].TreeHash == tree.Hash {
				return fmt.Errorf("nothing changed since last version (use 'drift add' after modifying files)")
			}
		}

		id := "v" + strconv.Itoa(branchCommitCount+1)
		author := core.Signature{
			Name:  sharedConfig.User.Name,
			Email: sharedConfig.User.Email,
		}
		commit := core.NewCommit(id, message, parentHash, branch, tree.Hash, author)

		// Issue 6: use atomic transaction to prevent orphan commits or
		// duplicate saves if one of the steps fails.
		emptyIdx := &core.Index{}
		if err := sharedStore.SaveCommitTransaction(commit, branch, emptyIdx); err != nil {
			return fmt.Errorf("failed to save commit: %w", err)
		}

		if message != "" {
			fmt.Printf("Saved version %s: %s\n", id, message)
		} else {
			fmt.Printf("Saved version %s\n", id)
		}

		fmt.Printf("\n  %d file(s) saved:\n", len(stagedPaths))
		for _, p := range stagedPaths {
			fmt.Printf("    %s\n", p)
		}
		return nil
	},
}

func init() {
	saveCmd.Flags().StringP("message", "m", "", "Version message")
	rootCmd.AddCommand(saveCmd)
}
