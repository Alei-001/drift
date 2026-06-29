package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/porcelain"
	"github.com/your-org/drift/storage"
	"github.com/zeebo/blake3"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify repository integrity",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			statusFailed("Check", ".drift/ directory not found.", "run 'drift init' first.")
			return err
		}
		defer store.Close()

		snapshots, err := store.ListSnapshots(&storage.ListOptions{})
		if err != nil {
			return err
		}

		// Collect unique chunk hashes
		hashSet := make(map[core.Hash]bool)
		for _, snap := range snapshots {
			for _, entry := range snap.Files {
				for _, hash := range entry.Chunks {
					hashSet[hash] = true
				}
			}
		}

		totalBlocks := len(hashSet)
		corrupt := 0
		missing := 0

		for hash := range hashSet {
			if !store.HasChunk(hash) {
				missing++
				continue
			}
			chunk, err := store.GetChunk(hash)
			if err != nil {
				corrupt++
				continue
			}
			computedHash := core.Hash(blake3.Sum256(chunk.Data))
			if computedHash != hash {
				corrupt++
			}
		}

		if missing == 0 && corrupt == 0 {
			statusOK("Check")
			fmt.Printf("%d blocks passed.\n", totalBlocks)
			return nil
		}

		statusWarn("Check")
		fmt.Printf("  blocks:  %d total, %d passed\n", totalBlocks, totalBlocks-corrupt-missing)
		fmt.Printf("  corrupt: %d\n", corrupt)
		fmt.Printf("  missing: %d\n", missing)
		fmt.Println()
		fmt.Println("  hint: corrupt chunks cannot be auto-repaired. Restore from a known-good snapshot.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
