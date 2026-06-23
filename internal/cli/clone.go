package cli

import (
	"fmt"
	"os"
	"path/filepath"

	driftsync "github.com/drift/drift/internal/sync"
	"github.com/spf13/cobra"
)

var cloneCmd = &cobra.Command{
	Use:   "clone <project> [destination]",
	Short: "Clone a project from the remote",
	Long: `Clone a project from the configured remote root into a new local directory.

The remote root must be set first with 'drift sync remote <path>'.

Examples:
  drift clone myproject              # clones into ./myproject
  drift clone myproject my-novel     # clones into ./my-novel`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if gcfg.RemoteRoot == "" {
			return fmt.Errorf("no remote root configured (run 'drift sync remote <path>' first)")
		}

		remoteName := args[0]
		destDir := remoteName
		if len(args) == 2 {
			destDir = args[1]
		}

		// Resolve destination relative to cwd if it's not absolute.
		if !filepath.IsAbs(destDir) {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			destDir = filepath.Join(cwd, destDir)
		}

		transport := driftsync.NewLocalTransport(gcfg.RemoteRoot)
		exists, err := transport.ProjectExists(remoteName)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("project %q not found on remote %s", remoteName, gcfg.RemoteRoot)
		}

		fmt.Printf("Cloning %s...\n", remoteName)
		if err := transport.Clone(remoteName, destDir); err != nil {
			return fmt.Errorf("clone failed: %w", err)
		}

		fmt.Printf("Cloned %s to %s\n", remoteName, destDir)
		fmt.Println("\nNext steps:")
		fmt.Printf("  cd %s\n", filepath.Base(destDir))
		fmt.Println("  drift log --all   # view history")
		fmt.Println("  drift status      # check working tree")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(cloneCmd)
}
