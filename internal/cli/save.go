package cli

import (
	"fmt"
	"strconv"

	"github.com/drift/drift/internal/config"
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

		builder := core.NewTreeBuilder(func(t *core.Tree) error {
			return sharedStore.PutTree(t)
		})

		tree, err := builder.BuildFromIndex(&idx)
		if err != nil {
			return fmt.Errorf("failed to build tree: %w", err)
		}

		commits, err := sharedStore.ListCommits()
		if err != nil {
			return fmt.Errorf("failed to list commits: %w", err)
		}

		parentHash := ""
		branch, _ := sharedStore.GetRef("HEAD")
		if branch == "" {
			branch = "main"
		}

		if len(commits) > 0 {
			if refHash, err := sharedStore.GetRef(branch); err == nil {
				parentHash = refHash
			}
		}

		cfg, _ := config.LoadConfig(sharedStore.DriftDir())
		if cfg == nil {
			cfg = config.DefaultConfig()
		}

		id := "v" + strconv.Itoa(len(commits)+1)
		author := core.Signature{
			Name:  cfg.User.Name,
			Email: cfg.User.Email,
		}
		commit := core.NewCommit(id, message, parentHash, branch, tree.Hash, author)

		if err := sharedStore.PutCommit(commit); err != nil {
			return fmt.Errorf("failed to store commit: %w", err)
		}

		if err := sharedStore.SaveRef(branch, commit.Hash); err != nil {
			return fmt.Errorf("failed to update ref: %w", err)
		}

		emptyIdx := &core.Index{}
		if err := sharedStore.SaveIndex(emptyIdx); err != nil {
			return fmt.Errorf("failed to clear index: %w", err)
		}

		if message != "" {
			fmt.Printf("Saved version %s: %s\n", id, message)
		} else {
			fmt.Printf("Saved version %s\n", id)
		}
		return nil
	},
}

func init() {
	saveCmd.Flags().StringP("message", "m", "", "Version message")
	rootCmd.AddCommand(saveCmd)
}
