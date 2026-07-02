package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/porcelain"
)

var statusShort bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show working tree status",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, cfg, err := openProjectOrReport("Status", cwd)
		if err != nil {
			return err
		}
		defer store.Close()

		changes, err := porcelain.DetectChanges(ctx, store, cwd, &cfg.Core)
		if err != nil {
			return err
		}

		total := len(changes.Added) + len(changes.Modified) + len(changes.Deleted)

		if total == 0 {
			statusOK("Status")
			fmt.Println("Nothing changed since last save.")
			return nil
		}

		if statusShort {
			fmt.Printf(">>> Status (%d %s)\n", total, pluralFile(total))
			for _, p := range changes.Added {
				fmt.Println(p)
			}
			for _, p := range changes.Modified {
				fmt.Println(p)
			}
			for _, p := range changes.Deleted {
				fmt.Println(p)
			}
		} else {
			header := fmt.Sprintf("Status (%d %s changed since last save)", total, pluralFile(total))
			fmt.Printf(">>> %s\n", header)
			fmt.Println()
			for _, p := range changes.Added {
				fmt.Printf("  +  %s\n", p)
			}
			for _, p := range changes.Modified {
				fmt.Printf("  ~  %s\n", p)
			}
			for _, p := range changes.Deleted {
				fmt.Printf("  -  %s\n", p)
			}
			summaryLine(total, len(changes.Added), len(changes.Modified), len(changes.Deleted))
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVarP(&statusShort, "short", "s", false, "short format, paths only")
	rootCmd.AddCommand(statusCmd)
}
