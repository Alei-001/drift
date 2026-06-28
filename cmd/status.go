package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/your-org/drift/util/fsutil"
)

var statusShort bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show working tree status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		index, err := store.GetIndex()
		if err != nil {
			return err
		}

		workspaceFiles := make(map[string]os.FileInfo)
		_ = fsutil.Walk(cwd, func(path string, info os.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(cwd, path)
			rel = filepath.ToSlash(rel)
			workspaceFiles[rel] = info
			return nil
		})

		type change struct {
			typ  string // "+", "~", "-"
			path string
		}
		var changes []change
		printed := make(map[string]bool)

		for _, entry := range index.Entries {
			if info, ok := workspaceFiles[entry.Path]; ok {
				if info.Size() != entry.Size || info.ModTime().Unix() != entry.ModTime {
					changes = append(changes, change{"~", entry.Path})
				}
				printed[entry.Path] = true
			} else {
				changes = append(changes, change{"-", entry.Path})
				printed[entry.Path] = true
			}
		}

		for path := range workspaceFiles {
			if !printed[path] {
				changes = append(changes, change{"+", path})
			}
		}

		if len(changes) == 0 {
			statusOK("Status")
			fmt.Println("Nothing changed since last save.")
			return nil
		}

		// Count by type
		var added, modified, deleted int
		for _, c := range changes {
			switch c.typ {
			case "+":
				added++
			case "~":
				modified++
			case "-":
				deleted++
			}
		}

		// Build header
		header := fmt.Sprintf("Status (%d files changed since last save)", len(changes))

		if statusShort {
			fmt.Printf(">>> Status (%d files)\n", len(changes))
			for _, c := range changes {
				fmt.Println(c.path)
			}
		} else {
			fmt.Printf(">>> %s\n", header)
			fmt.Println()
			for _, c := range changes {
				fmt.Printf("  %s  %s\n", c.typ, c.path)
			}
			summaryLine(len(changes), added, modified, deleted)
		}
		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVarP(&statusShort, "short", "s", false, "short format, paths only")
	rootCmd.AddCommand(statusCmd)
}
