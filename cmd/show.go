package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/your-org/drift/util/pathutil"
)

var showOpen bool

var showCmd = &cobra.Command{
	Use:   "show <snapshot-id> <file>",
	Short: "Show file content from a snapshot",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		idStr := args[0]
		filePath := args[1]

		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		snapshot := resolveSnapshot(store, idStr)
		if snapshot == nil {
			statusFailed("Show", fmt.Sprintf("snapshot not found: %s.", idStr), "use 'drift log' to list available snapshots.")
			return fmt.Errorf("snapshot not found: %s", idStr)
		}

		var targetEntry *core.FileEntry
		normalizedPath := pathutil.Normalize(filePath)
		for i := range snapshot.Files {
			if snapshot.Files[i].Path == normalizedPath {
				targetEntry = &snapshot.Files[i]
				break
			}
		}
		if targetEntry == nil {
			statusFailed("Show", fmt.Sprintf("'%s' not found in snapshot %s.", filePath, snapshot.ShortID()),
				fmt.Sprintf("use 'drift log -v %s' to list files in this snapshot.", snapshot.ShortID()))
			return fmt.Errorf("file not found in snapshot: %s", filePath)
		}

		var data []byte
		for _, hash := range targetEntry.Chunks {
			chunk, err := store.GetChunk(hash)
			if err != nil {
				return fmt.Errorf("missing chunk %s: %w", hash.String(), err)
			}
			data = append(data, chunk.Data...)
		}

		header := data
		if len(header) > 512 {
			header = header[:512]
		}
		engine := filetype.DetectEngine(filePath, header)

		// Binary file handling
		if engine != nil && engine.Name() != "text" {
			if showOpen {
				return openExternal(snapshot, filePath, data)
			}
			// Show metadata
			fmt.Printf(">>> File %s:%s\n", snapshot.ShortID(), filePath)
			fmt.Printf("  Size:       %s\n", formatSize(targetEntry.Size))
			if targetEntry.ModTime > 0 {
				modTimeStr := time.Unix(targetEntry.ModTime, 0).Format("01-02 15:04")
				fmt.Printf("  Modified:   %s\n", modTimeStr)
			}
			fmt.Println()
			fmt.Println("  hint: use --open to view with system program.")
			return nil
		}

		// Text file: print header then content
		fmt.Printf(">>> File %s:%s\n", snapshot.ShortID(), filePath)
		fmt.Println()
		os.Stdout.Write(data)
		return nil
	},
}

func openExternal(snapshot *core.Snapshot, filePath string, data []byte) error {
	fmt.Printf(">>> Opening [ok]\n")
	// Write temp file and open
	tmpFile, err := os.CreateTemp("", "drift-show-*"+filepathExt(filePath))
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		os.Remove(tmpPath)
		return err
	}
	tmpFile.Close()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", tmpPath)
	case "darwin":
		cmd = exec.Command("open", tmpPath)
	default:
		cmd = exec.Command("xdg-open", tmpPath)
	}
	if err := cmd.Start(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	fmt.Printf("Launched system viewer for %s:%s.\n", snapshot.ShortID(), filePath)
	return nil
}

func filepathExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
		if path[i] == '/' || path[i] == '\\' {
			return ""
		}
	}
	return ""
}

func init() {
	showCmd.Flags().BoolVar(&showOpen, "open", false, "open file with system viewer")
	rootCmd.AddCommand(showCmd)
}

func resolveSnapshot(store storage.Storer, id string) *core.Snapshot {
	if id == "HEAD" {
		headRef, err := store.GetRef("HEAD")
		if err != nil {
			return nil
		}
		snap, err := store.GetSnapshot(core.SnapshotID{Hash: headRef.Target})
		if err != nil {
			return nil
		}
		return snap
	}

	// Full hash (64 chars)
	if len(id) == 64 {
		var hash core.Hash
		for i := 0; i < 32; i++ {
			b, ok := parseHexByte(id[i*2 : i*2+2])
			if !ok {
				return nil
			}
			hash[i] = b
		}
		snap, err := store.GetSnapshot(core.SnapshotID{Hash: hash})
		if err != nil {
			return nil
		}
		return snap
	}

	// Short hash prefix (or tag name)
	snapshots, err := store.ListSnapshots(&storage.ListOptions{})
	if err != nil {
		return nil
	}
	for _, s := range snapshots {
		if strings.HasPrefix(s.ShortID(), id) || strings.HasPrefix(s.FullID(), id) {
			return s
		}
	}
	return nil
}
