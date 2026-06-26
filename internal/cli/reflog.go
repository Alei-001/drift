package cli

import (
	"fmt"
	"strings"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

const nullHash = "0000000000000000000000000000000000000000000000000000000000000000"

// NewReflogCmd creates the reflog subcommand.
func NewReflogCmd(application *apppkg.App) *cobra.Command {
	var (
		number    int
		porcelain bool
		verbose   bool
	)

	cmd := &cobra.Command{
		Use:   "reflog",
		Short: "Show operation log (undo/redo history)",
		RunE: func(cmd *cobra.Command, args []string) error {
			operations, err := application.Log(number)
			if err != nil {
				return err
			}

			if porcelain {
				formatOperationsPorcelain(operations)
			} else {
				formatOperations(operations, verbose)
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&number, "number", "n", 20, "Limit number of operations (0 = all)")
	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "Machine-readable output")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output (show ref details)")

	return cmd
}

// formatOperations displays operations in human-readable format.
func formatOperations(operations []apppkg.OperationEntry, verbose bool) {
	descWidth := 20
	for _, op := range operations {
		if len(op.Desc) > descWidth {
			descWidth = len(op.Desc)
		}
	}
	if descWidth > 60 {
		descWidth = 60
	}

	fmt.Printf("%-19s  %-6s  %-*s\n", "DATE", "OP", descWidth, "DESCRIPTION")
	for _, op := range operations {
		desc := op.Desc
		if len(desc) > descWidth {
			desc = desc[:descWidth-3] + "..."
		}
		fmt.Printf("%-19s  %-6s  %s\n", op.Timestamp.Format("2006-01-02 15:04:05"), op.Op, desc)
		if verbose {
			for _, change := range op.RefChanges {
				fmt.Printf("  %s: %s → %s\n", change.Ref, shortRef(change.Before), shortRef(change.After))
			}
			for _, e := range op.IndexSnapshot {
				fmt.Printf("  %s %s\n", e.Path, e.Hash[:8])
			}
		}
	}
}

// formatOperationsPorcelain displays operations in machine-readable format.
func formatOperationsPorcelain(operations []apppkg.OperationEntry) {
	for _, op := range operations {
		fmt.Printf("timestamp %s\n", op.Timestamp.Format("2006-01-02T15:04:05Z07:00"))
		fmt.Printf("op %s\n", op.Op)
		fmt.Printf("desc %s\n", op.Desc)
		for _, change := range op.RefChanges {
			before := change.Before
			if before == "" {
				before = nullHash
			}
			after := change.After
			if after == "" {
				after = nullHash
			}
			fmt.Printf("ref %s %s %s\n", change.Ref, before, after)
		}
		for _, e := range op.IndexSnapshot {
			fmt.Printf("index %s %s\n", e.Path, e.Hash)
		}
		fmt.Println()
	}
}

func shortRef(v string) string {
	if v == "" {
		return strings.Repeat("-", core.CommitIDLen)
	}
	if len(v) > core.CommitIDLen {
		return v[:core.CommitIDLen]
	}
	return v
}
