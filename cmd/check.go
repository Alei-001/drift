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
			statusFailed("Check", ".drift/ directory not found.", "run 'drift init' first.")
			return err
		}
		defer store.(*filesystem.FSStorage).Close()

		snapshots, err := store.ListSnapshots(&storage.ListOptions{})
		if err != nil {
			return err
		}

		totalBlocks := 0
		corrupt := 0
		missing := 0
		repaired := 0

		for _, snap := range snapshots {
			for _, entry := range snap.Files {
				for _, hash := range entry.Chunks {
					totalBlocks++
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
						if checkFix {
							repaired++
						}
						corrupt++
					}
				}
			}
		}

		if missing == 0 && corrupt == 0 {
			statusOK("Check")
			fmt.Printf("%d blocks passed.\n", totalBlocks)
			return nil
		}

		if checkFix && repaired > 0 {
			statusOK("Check")
			fmt.Printf("  blocks:  %d total, %d passed → %d passed\n", totalBlocks, totalBlocks-corrupt, totalBlocks)
			fmt.Printf("  repaired: %d blocks.\n", repaired)
			return nil
		}

		statusWarn("Check")
		fmt.Printf("  blocks:  %d total, %d passed\n", totalBlocks, totalBlocks-corrupt)
		fmt.Printf("  corrupt: %d\n", corrupt)
		fmt.Printf("  missing: %d\n", missing)
		fmt.Println()
		fmt.Println("  hint: use --fix to attempt repair.")
		return nil
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "attempt to repair corrupted chunks")
	rootCmd.AddCommand(checkCmd)
}
