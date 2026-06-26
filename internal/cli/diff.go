package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	apppkg "github.com/drift/drift/internal/app"
	"github.com/drift/drift/internal/core"
	"github.com/spf13/cobra"
)

// NewDiffCmd creates the diff subcommand.
func NewDiffCmd(application *apppkg.App) *cobra.Command {
	var (
		version1 string
		version2 string
		patch    bool
		output   string
	)

	cmd := &cobra.Command{
		Use:   "diff [<version1>] [<version2>] [-- <path>...]",
		Short: "Show differences between versions",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse paths after -- separator
			var paths []string
			for i, arg := range args {
				if arg == "--" {
					paths = args[i+1:]
					args = args[:i]
					break
				}
			}

			// Parse version arguments
			if len(args) > 0 {
				version1 = args[0]
			}
			if len(args) > 1 {
				version2 = args[1]
			}

			// --from/--to flags override positional args
			if cmd.Flags().Changed("from") {
				version1 = cmd.Flag("from").Value.String()
			}
			if cmd.Flags().Changed("to") {
				version2 = cmd.Flag("to").Value.String()
			}

			result, err := application.Diff(apppkg.DiffOptions{
				V1:    version1,
				V2:    version2,
				Paths: paths,
			})
			if err != nil {
				return err
			}

			if len(result.Entries) == 0 {
				fmt.Println("No changes")
				return nil
			}

			// --patch or --output: produce unified diff format.
			if patch || output != "" {
				patchOutput := formatPatch(result.Entries)
				if output != "" {
					if err := os.WriteFile(output, []byte(patchOutput), 0644); err != nil {
						return fmt.Errorf("failed to write patch file: %w", err)
					}
					fmt.Printf("Patch written to %s (%d file(s))\n", output, len(result.Entries))
				} else {
					fmt.Print(patchOutput)
				}
				return nil
			}

			// Default: summary listing
			fmt.Printf("Changed %d file(s):\n", len(result.Entries))
			for _, e := range result.Entries {
				switch e.Status {
				case "added":
					fmt.Printf("  + %s\n", e.Path)
				case "modified":
					fmt.Printf("  M %s\n", e.Path)
				case "deleted":
					fmt.Printf("  - %s\n", e.Path)
				default:
					fmt.Printf("  %s %s\n", e.Status, e.Path)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&version1, "from", "", "Source version")
	cmd.Flags().StringVar(&version2, "to", "", "Target version")
	cmd.Flags().BoolVarP(&patch, "patch", "p", false, "Show unified diff output")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write patch to file")

	return cmd
}

// formatPatch produces a unified diff format string from diff entries.
func formatPatch(entries []apppkg.DiffEntry) string {
	var buf strings.Builder

	for _, e := range entries {
		if e.IsBinary {
			fmt.Fprintf(&buf, "Binary files differ: %s\n\n", e.Path)
			continue
		}

		switch e.Status {
		case "added":
			fmt.Fprintf(&buf, "--- /dev/null\n")
			fmt.Fprintf(&buf, "+++ %s\n", e.Path)
			for _, edit := range e.Edits {
				switch edit.Op {
				case core.DiffInsert:
					fmt.Fprintf(&buf, "+%s\n", edit.Line)
				case core.DiffKeep:
					fmt.Fprintf(&buf, " %s\n", edit.Line)
				}
			}
		case "deleted":
			fmt.Fprintf(&buf, "--- %s\n", e.Path)
			fmt.Fprintf(&buf, "+++ /dev/null\n")
			for _, edit := range e.Edits {
				switch edit.Op {
				case core.DiffDelete:
					fmt.Fprintf(&buf, "-%s\n", edit.Line)
				case core.DiffKeep:
					fmt.Fprintf(&buf, " %s\n", edit.Line)
				}
			}
		case "modified":
			fmt.Fprintf(&buf, "--- %s\n", e.Path)
			fmt.Fprintf(&buf, "+++ %s\n", e.Path)
			for _, edit := range e.Edits {
				switch edit.Op {
				case core.DiffKeep:
					fmt.Fprintf(&buf, " %s\n", edit.Line)
				case core.DiffDelete:
					fmt.Fprintf(&buf, "-%s\n", edit.Line)
				case core.DiffInsert:
					fmt.Fprintf(&buf, "+%s\n", edit.Line)
				}
			}
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

// Ensure bufio is used (for potential future interactive features).
var _ = bufio.NewReader
