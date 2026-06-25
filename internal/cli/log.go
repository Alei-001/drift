package cli

import (
	"fmt"

	"github.com/drift/drift/internal/repo"
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

// logLimit is the default number of entries shown by `drift log`.
// A value of 0 means "show all entries".
var logLimit int

// logPorcelain switches `drift log` to machine-readable output.
var logPorcelain bool

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show recent operations (for undo reference)",
	Long: `Show recent operations that modified repository state, newest first.
Useful as a safety net before running undo.

By default, shows the 20 most recent operations. Use -n to change the limit,
or -n 0 to show all entries.

Examples:
  drift log              # show recent operations (default 20)
  drift log -n 10        # show 10 most recent operations
  drift log -n 0         # show all operations
  drift log --porcelain  # machine-readable output`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := sharedRepo.ReadOperations()
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No operations recorded yet")
			return nil
		}

		// limit == 0 means show all; otherwise cap at limit.
		limit := logLimit
		if limit == 0 {
			limit = len(entries)
		}
		if len(entries) < limit {
			limit = len(entries)
		}

		if logPorcelain {
			printOperationsPorcelain(entries, limit)
			return nil
		}

		fmt.Printf("Recent operations (newest first):\n\n")

		for i := 0; i < limit; i++ {
			e := entries[i]
			fmt.Printf("  %d. %s  %s  %s\n", i+1, e.Timestamp.Format("2006-01-02 15:04:05"), e.Op, e.Desc)
		}

		shown := limit
		if logLimit == 0 {
			// All entries shown.
		} else if len(entries) > shown {
			fmt.Printf("\n(%d more older operations — not shown, use -n 0 to show all)\n", len(entries)-shown)
		}

		fmt.Println("\nTo undo the most recent operation: drift undo")
		return nil
	},
}

// printOperationsPorcelain outputs operations in a machine-readable format:
// <index>\t<timestamp>\t<op>\t<desc>
func printOperationsPorcelain(entries []repo.OperationEntry, limit int) {
	for i := 0; i < limit; i++ {
		e := entries[i]
		fmt.Printf("%d\t%s\t%s\t%s\n", i+1, e.Timestamp.Format("2006-01-02 15:04:05"), e.Op, e.Desc)
	}
}

func init() {
	logCmd.Flags().IntVarP(&logLimit, "number", "n", 20, "Number of entries to show (0 = all)")
	logCmd.Flags().BoolVar(&logPorcelain, "porcelain", false, "Machine-readable output")
	rootCmd.AddCommand(logCmd)
}
