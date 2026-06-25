package cli

import (
	"fmt"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/spf13/cobra"
)

// NewLogCmd creates the log subcommand.
func NewLogCmd(application *apppkg.App) *cobra.Command {
	var (
		number    int
		porcelain bool
	)

	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show commit log",
		RunE: func(cmd *cobra.Command, args []string) error {
			operations, err := application.Log(number)
			if err != nil {
				return err
			}

			if porcelain {
				formatOperationsPorcelain(operations)
			} else {
				formatOperations(operations)
			}
			return nil
		},
	}

	cmd.Flags().IntVarP(&number, "number", "n", 20, "Limit number of operations (0 = all)")
	cmd.Flags().BoolVar(&porcelain, "porcelain", false, "Machine-readable output")

	return cmd
}

// formatOperations displays operations in human-readable format.
func formatOperations(operations []apppkg.OperationEntry) {
	for _, op := range operations {
		fmt.Printf("%s %s %s\n", op.Timestamp.Format("2006-01-02 15:04:05"), op.Op, op.Desc)
		if len(op.RefChanges) > 0 {
			for _, change := range op.RefChanges {
				fmt.Printf("  %s: %s -> %s\n", change.Ref, change.Before, change.After)
			}
		}
		fmt.Println()
	}
}

// formatOperationsPorcelain displays operations in machine-readable format.
func formatOperationsPorcelain(operations []apppkg.OperationEntry) {
	for _, op := range operations {
		fmt.Printf("timestamp %s\n", op.Timestamp.Format("2006-01-02T15:04:05Z07:00"))
		fmt.Printf("op %s\n", op.Op)
		fmt.Printf("desc %s\n", op.Desc)
		for _, change := range op.RefChanges {
			fmt.Printf("ref %s %s %s\n", change.Ref, change.Before, change.After)
		}
		fmt.Println()
	}
}
