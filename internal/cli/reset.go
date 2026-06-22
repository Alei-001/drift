package cli

import (
	"fmt"

	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Unstage all staged changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		idx := &core.Index{}
		if err := sharedStore.SaveIndex(idx); err != nil {
			return fmt.Errorf("failed to reset index: %w", err)
		}

		fmt.Println("Staging area cleared")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resetCmd)
}
