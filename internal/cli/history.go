package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Operation log records user actions that modify repository state, enabling
// an undo safety net. This is a friendly version of Git's reflog — instead
// of exposing HEAD@{1} syntax, users see a readable history and can undo
// the most recent operation.
//
// Storage: .drift/operations.log (append-only JSON lines)
//
// Each entry records:
//   - Timestamp
//   - Operation type (save, switch, branch-delete, branch-rename, restore)
//   - Description (human-readable)
//   - Before/after state of affected refs

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show recent operations (for undo reference)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := sharedRepo.ReadOperations()
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No operations recorded yet")
			return nil
		}

		fmt.Printf("Recent operations (newest first):\n\n")
		limit := 20
		if len(entries) < limit {
			limit = len(entries)
		}
		for i := 0; i < limit; i++ {
			e := entries[i]
			fmt.Printf("  %d. %s  %s  %s\n", i+1, e.Timestamp.Format("2006-01-02 15:04:05"), e.Op, e.Desc)
		}

		if len(entries) > 20 {
			fmt.Printf("\n(%d more older operations — not shown)\n", len(entries)-20)
		}

		fmt.Println("\nTo undo the most recent operation: drift undo")
		return nil
	},
}

var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Undo the most recent operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		last, restored, err := sharedRepo.Undo()
		if err != nil {
			return err
		}

		fmt.Printf("Undid: %s (%s)\n", last.Desc, last.Op)
		fmt.Printf("Restored %d ref(s) to previous state.\n", restored)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(undoCmd)
}
