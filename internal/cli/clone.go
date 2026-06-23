package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	driftsync "github.com/drift/drift/internal/sync"
	"github.com/spf13/cobra"
)

var cloneCmd = &cobra.Command{
	Use:   "clone <project> [destination]",
	Short: "Clone a project from the remote",
	Long: `Clone a project from the configured remote into a new local directory.

The remote must be set first with 'drift sync remote <path-or-url>'.

Examples:
  drift clone myproject              # clones into ./myproject
  drift clone myproject my-novel     # clones into ./my-novel`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		gcfg, err := driftsync.LoadGlobalConfig()
		if err != nil {
			return err
		}
		if gcfg.GetRemoteType() == driftsync.RemoteNone {
			return fmt.Errorf("no remote configured (run 'drift sync remote <path-or-url>' first)")
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

		// Check if project exists on remote.
		switch gcfg.GetRemoteType() {
		case driftsync.RemoteLocal:
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

		case driftsync.RemoteWebDAV:
			// For WebDAV, we use the sync engine to pull all files.
			if err := cloneFromWebDAV(gcfg, remoteName, destDir); err != nil {
				return err
			}
		}

		fmt.Printf("Cloned %s to %s\n", remoteName, destDir)
		fmt.Println("\nNext steps:")
		fmt.Printf("  cd %s\n", filepath.Base(destDir))
		fmt.Println("  drift log --all   # view history")
		fmt.Println("  drift status      # check working tree")
		return nil
	},
}

// cloneFromWebDAV clones a project from a WebDAV remote by listing all files
// and downloading them.
func cloneFromWebDAV(gcfg *driftsync.GlobalConfig, remoteName, destDir string) error {
	// Check destination is empty or doesn't exist.
	if info, err := os.Stat(destDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("destination %q exists and is not a directory", destDir)
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("destination %q is not empty", destDir)
		}
	}

	transport := driftsync.ProjectTransportForConfig(gcfg, remoteName)
	if transport == nil {
		return fmt.Errorf("no transport available")
	}

	// List all files on the remote.
	files, err := transport.List("")
	if err != nil {
		return fmt.Errorf("failed to list remote files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("project %q not found or empty on remote", remoteName)
	}

	fmt.Printf("Cloning %s...\n", remoteName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, remotePath := range files {
		localPath := filepath.Join(destDir, filepath.FromSlash(remotePath))
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return err
		}
		f, err := os.Create(localPath)
		if err != nil {
			return err
		}
		if err := transport.Get(remotePath, f); err != nil {
			f.Close()
			return fmt.Errorf("failed to download %s: %w", remotePath, err)
		}
		f.Close()
	}

	return nil
}

// Ensure io is used (for future streaming improvements).
var _ = io.Copy
var _ = strings.TrimSpace

func init() {
	rootCmd.AddCommand(cloneCmd)
}
