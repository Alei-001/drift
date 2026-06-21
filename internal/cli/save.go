package cli

import (
	"fmt"
	"os"
	"strconv"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var saveCmd = &cobra.Command{
	Use:   "save -m <message>",
	Short: "Save staged changes as a new version",
	RunE: func(cmd *cobra.Command, args []string) error {
		message, _ := cmd.Flags().GetString("message")
		if message == "" {
			return fmt.Errorf("message is required (use -m flag)")
		}

		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		store := storage.NewStore(dir)
		if !store.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}

		var idx core.Index
		if err := store.LoadIndex(&idx); err != nil {
			return fmt.Errorf("failed to load index: %w", err)
		}

		if len(idx.Entries) == 0 {
			return fmt.Errorf("nothing to save (use 'drift add' first)")
		}

		builder := core.NewTreeBuilder(func(t *core.Tree) error {
			return store.PutTree(t)
		})

		tree, err := builder.BuildFromIndex(&idx)
		if err != nil {
			return fmt.Errorf("failed to build tree: %w", err)
		}

		commits, err := store.ListCommits()
		if err != nil {
			return fmt.Errorf("failed to list commits: %w", err)
		}

		parentHash := ""
		branch := "main"

		if len(commits) > 0 {
			latest := commits[len(commits)-1]
			parentHash = latest.Hash
			branch = latest.Branch
		}

		id := "v" + strconv.Itoa(len(commits)+1)
		commit := core.NewCommit(id, message, parentHash, branch, tree.Hash)

		if err := store.PutCommit(commit); err != nil {
			return fmt.Errorf("failed to store commit: %w", err)
		}

		if err := store.SaveRef(branch, commit.Hash); err != nil {
			return fmt.Errorf("failed to update ref: %w", err)
		}

		emptyIdx := &core.Index{}
		if err := store.SaveIndex(emptyIdx); err != nil {
			return fmt.Errorf("failed to clear index: %w", err)
		}

		fmt.Printf("Saved version %s: %s\n", id, message)
		return nil
	},
}

func init() {
	saveCmd.Flags().StringP("message", "m", "", "Version message")
	rootCmd.AddCommand(saveCmd)
}
