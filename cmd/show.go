package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
)

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
			return fmt.Errorf("snapshot not found: %s", idStr)
		}

		var targetEntry *core.FileEntry
		for i := range snapshot.Files {
			if snapshot.Files[i].Path == filePath {
				targetEntry = &snapshot.Files[i]
				break
			}
		}
		if targetEntry == nil {
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
		if engine != nil && engine.Name() != "text" {
			fmt.Printf("[binary file: %s, %d bytes]\n", filePath, len(data))
			return nil
		}

		os.Stdout.Write(data)
		return nil
	},
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

	// Short hash prefix
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

func parseHexByte(s string) (byte, bool) {
	if len(s) != 2 {
		return 0, false
	}
	var b byte
	for i := 0; i < 2; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			b = b<<4 | (c - '0')
		case c >= 'a' && c <= 'f':
			b = b<<4 | (c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			b = b<<4 | (c - 'A' + 10)
		default:
			return 0, false
		}
	}
	return b, true
}

func init() {
	rootCmd.AddCommand(showCmd)
}
