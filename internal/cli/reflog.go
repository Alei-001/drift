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
	const maxMsgLen = 40
	const dateWidth = 19     // "2006-01-02 15:04:05"
	const opWidth = 8        // longest op: "restore"
	const sep = "    "       // spacing between columns

	descs := make([]string, len(operations))
	for i, op := range operations {
		descs[i] = truncateParensMessage(op.Desc, maxMsgLen)
	}

	descWidth := 20
	for _, d := range descs {
		if len(d) > descWidth {
			descWidth = len(d)
		}
	}
	if descWidth > 60 {
		descWidth = 60
	}

	// Header: format plain text at correct width, then colorize.
	fmt.Printf("%s%s%s%s%s\n",
		colorCyan(fmt.Sprintf("%-*s", dateWidth, "DATE")),
		sep,
		colorCyan(fmt.Sprintf("%-*s", opWidth, "OP")),
		sep,
		colorCyan(fmt.Sprintf("%-*s", descWidth, "DESCRIPTION")))

	for i, op := range operations {
		desc := descs[i]
		if len(desc) > descWidth {
			desc = desc[:descWidth-3] + "..."
		}
		fmt.Printf("%s%s%s%s%s\n",
			fmt.Sprintf("%-*s", dateWidth, op.Timestamp.Format("2006-01-02 15:04:05")),
			sep,
			colorYellow(fmt.Sprintf("%-*s", opWidth, string(op.Op))),
			sep,
			fmt.Sprintf("%-*s", descWidth, desc))
		if verbose {
			for _, change := range op.RefChanges {
				fmt.Printf("  %s: %s → %s\n", colorGray(change.Ref), colorGray(shortRef(change.Before)), colorGray(shortRef(change.After)))
			}
			for _, e := range op.IndexSnapshot {
				fmt.Printf("  %s %s\n", colorGray(e.Path), colorGray(shortHash(e.Hash)))
			}
		}
	}
}

func truncateParensMessage(desc string, maxMsg int) string {
	start := strings.Index(desc, " (")
	if start == -1 {
		return desc
	}
	end := strings.Index(desc[start+2:], ")")
	if end == -1 {
		return desc
	}
	end += start + 2
	msg := desc[start+2 : end]
	if len(msg) > maxMsg {
		msg = msg[:maxMsg-3] + "..."
	}
	return desc[:start+2] + msg + desc[end:]
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
