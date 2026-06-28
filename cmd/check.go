package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
	"github.com/zeebo/blake3"
)

var checkFix bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify repository integrity",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		snapshots, err := store.ListSnapshots(&storage.ListOptions{})
		if err != nil {
			return err
		}

		errors := 0
		for _, snap := range snapshots {
			for _, entry := range snap.Files {
				for _, hash := range entry.Chunks {
					if !store.HasChunk(hash) {
						fmt.Printf("ERROR: missing chunk %s (file: %s, snapshot: %s)\n", hash.String(), entry.Path, snap.ShortID())
						errors++
					} else {
						chunk, err := store.GetChunk(hash)
						if err != nil {
							fmt.Printf("ERROR: cannot read chunk %s (file: %s, snapshot: %s): %v\n", hash.String(), entry.Path, snap.ShortID(), err)
							errors++
							continue
						}
						computedHash := core.Hash(blake3.Sum256(chunk.Data))
						if computedHash != hash {
							if checkFix {
								fmt.Printf("ERROR: corrupted chunk %s (file: %s, snapshot: %s) — cannot fix (no redundancy available)\n", hash.String(), entry.Path, snap.ShortID())
							} else {
								fmt.Printf("ERROR: corrupted chunk %s (file: %s, snapshot: %s)\n", hash.String(), entry.Path, snap.ShortID())
							}
							errors++
						}
					}
				}
			}
		}

		if errors == 0 {
			fmt.Println("Repository integrity check passed.")
		} else {
			fmt.Printf("Found %d errors.\n", errors)
		}
		return nil
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "attempt to repair corrupted chunks")
	rootCmd.AddCommand(checkCmd)
}
