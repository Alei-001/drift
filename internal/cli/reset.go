package cli

import (
	"fmt"
	"os"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Unstage all staged changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		store := storage.NewStore(dir)
		if !store.IsInitialized() {
			return fmt.Errorf("not a drift project (run 'drift init')")
		}

		idx := &core.Index{}
		if err := store.SaveIndex(idx); err != nil {
			return fmt.Errorf("failed to reset index: %w", err)
		}

		fmt.Println("Staging area cleared")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resetCmd)
}
