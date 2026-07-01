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
		ctx := cmd.Context()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store, _, err := porcelain.OpenProject(cwd)
		if err != nil {
			statusFailed("Check", ".drift/ directory not found.", "run 'drift init' first.")
		return ErrSilent
		}
		defer store.Close()

		snapshots, err := store.ListSnapshots(ctx, &storage.ListOptions{})
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
			if !store.HasChunk(ctx, hash) {
				missing++
				continue
			}
			chunk, err := store.GetChunk(ctx, hash)
			if err != nil {
				corrupt++
				continue
			}
			computedHash := core.Hash(blake3.Sum256(chunk.Data))
			if computedHash != hash {
				corrupt++
			}
		}

		// Verify snapshot integrity (BLAKE3 hash of serialized proto).
		snapshotCorrupt := 0
		for _, snap := range snapshots {
			// Re-fetch via GetSnapshot which performs integrity verification.
			if _, err := store.GetSnapshot(ctx, snap.ID); err != nil {
				snapshotCorrupt++
				continue
			}
		}

		if missing == 0 && corrupt == 0 && snapshotCorrupt == 0 {
			statusOK("Check")
			fmt.Printf("%d blocks passed.\n", totalBlocks)

			// 追加不可达快照提示
			unreachable, err := porcelain.CountUnreachableSnapshots(ctx, store)
			if err == nil && unreachable > 0 {
				fmt.Printf("  hint: %d unreachable snapshots detected. use 'drift gc --dry-run' to review.\n", unreachable)
			}

			return nil
		}

		statusWarn("Check")
		fmt.Printf("  blocks:  %d total, %d passed\n", totalBlocks, totalBlocks-corrupt-missing)
		fmt.Printf("  corrupt: %d\n", corrupt)
		fmt.Printf("  missing: %d\n", missing)
		if snapshotCorrupt > 0 {
			fmt.Printf("  snapshots: %d corrupt\n", snapshotCorrupt)
		}
		fmt.Println()
		if corrupt > 0 {
			fmt.Println("  hint: corrupt chunks cannot be auto-repaired. Restore affected files from a known-good snapshot using 'drift restore <id>'.")
		}
		if missing > 0 {
			fmt.Println("  hint: missing chunks indicate data loss. Restore from a known-good snapshot using 'drift restore <id>'.")
		}
		if snapshotCorrupt > 0 {
			fmt.Println("  hint: corrupt snapshots have damaged metadata. Use 'drift gc' to clean up unreachable snapshots.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
