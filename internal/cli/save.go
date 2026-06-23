package cli

import (
	"fmt"
	"strconv"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var saveCmd = &cobra.Command{
	Use:   "save [-m message] [--amend]",
	Short: "Save staged changes as a new version",
	RunE: func(cmd *cobra.Command, args []string) error {
		message, _ := cmd.Flags().GetString("message")
		amend, _ := cmd.Flags().GetBool("amend")

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
			if branchCommits[0].TreeHash == tree.Hash && !amend {
				return fmt.Errorf("nothing changed since last version (use 'drift add' after modifying files)")
			}
		}

		// --amend: replace the most recent commit instead of creating a new one.
		if amend {
			if branchCommitCount == 0 {
				return fmt.Errorf("no version to amend (create one first with 'drift save')")
			}
			lastCommit := branchCommits[0]
			// Use the same version ID and parent, but new tree and message.
			id := lastCommit.ID
			// If no new message provided, keep the original.
			if message == "" {
				message = lastCommit.Message
			}
			parentHash = lastCommit.Parent

			author := core.Signature{
				Name:  sharedConfig.User.Name,
				Email: sharedConfig.User.Email,
			}
			commit := core.NewCommit(id, message, parentHash, branch, tree.Hash, author)

			prevBranchHash := lastCommit.Hash
			emptyIdx := &core.Index{}
			if err := sharedStore.SaveCommitTransaction(commit, branch, emptyIdx); err != nil {
				return fmt.Errorf("failed to save amended commit: %w", err)
			}

			recordOperation(sharedStore, OpSave, fmt.Sprintf("amend %s on %s", id, branch), []RefChange{
				{Ref: branch, Before: prevBranchHash, After: commit.Hash},
			})

			fmt.Printf("Amended version %s: %s\n", id, message)
			fmt.Printf("\n  %d file(s) in amended version:\n", len(stagedPaths))
			for _, p := range stagedPaths {
				fmt.Printf("    %s\n", p)
			}
			return nil
		}

		id := "v" + strconv.Itoa(branchCommitCount+1)
		author := core.Signature{
			Name:  sharedConfig.User.Name,
			Email: sharedConfig.User.Email,
		}
		commit := core.NewCommit(id, message, parentHash, branch, tree.Hash, author)

		// Issue 6: use atomic transaction to prevent orphan commits or
		// duplicate saves if one of the steps fails.
		// Capture the branch's previous commit hash for undo.
		prevBranchHash := ""
		if branchCommitCount > 0 {
			prevBranchHash = branchCommits[0].Hash
		}
		emptyIdx := &core.Index{}
		if err := sharedStore.SaveCommitTransaction(commit, branch, emptyIdx); err != nil {
			return fmt.Errorf("failed to save commit: %w", err)
		}

		// Record operation for undo.
		desc := fmt.Sprintf("save %s on %s", id, branch)
		if message != "" {
			desc = fmt.Sprintf("save %s (%s) on %s", id, message, branch)
		}
		recordOperation(sharedStore, OpSave, desc, []RefChange{
			{Ref: branch, Before: prevBranchHash, After: commit.Hash},
		})

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
	saveCmd.Flags().Bool("amend", false, "Amend the most recent version instead of creating a new one")
	rootCmd.AddCommand(saveCmd)
}
