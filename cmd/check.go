package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/porcelain"
)

var (
	checkVerbose bool
	checkFilter  string
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify repository integrity",
	Long:  "Verify the integrity of all chunks in the .drift/ directory by recomputing their BLAKE3 hashes.",
	RunE:  runCheck,
}

func init() {
	checkCmd.Flags().BoolVar(&checkVerbose, "verbose", false, "show per-chunk results")
	checkCmd.Flags().StringVar(&checkFilter, "filter", "", "only verify files matching the glob pattern")
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cwd, err := getCwd(cmd)
	if err != nil {
		return err
	}
	store, _, err := openProjectOrReport("Check", "check", cwd)
	if err != nil {
		return err
	}
	defer store.Close()

	report, err := porcelain.VerifyIntegrity(ctx, store, cwd, checkFilter, checkVerbose)
	if err != nil {
		reportFailed("Check", "check", err.Error(), "")
		return ErrSilent
	}

	if globalJSON {
		status := "ok"
		var hintParts []string
		if report.Corrupt > 0 || report.Missing > 0 || report.SnapshotCorrupt > 0 {
			status = "warning"
			if report.Corrupt > 0 {
				hintParts = append(hintParts, "corrupt chunks cannot be auto-repaired. Restore affected files from a known-good snapshot using 'drift restore <version>'.")
			}
			if report.Missing > 0 {
				hintParts = append(hintParts, "missing chunks indicate data loss. Restore from a known-good snapshot using 'drift restore <version>'.")
			}
			if report.SnapshotCorrupt > 0 {
				hintParts = append(hintParts, "corrupt snapshots have damaged metadata. Use 'drift gc' to clean up unreachable snapshots.")
			}
		} else {
			unreachable, uerr := porcelain.CountUnreachableSnapshots(ctx, store, cwd)
			if uerr == nil && unreachable > 0 {
				hintParts = append(hintParts, fmt.Sprintf("%d unreachable snapshots detected. use 'drift gc --dry-run' to review.", unreachable))
			}
		}
		if err := outputJSON(JSONEnvelope{
			Command: "check",
			Status:  status,
			Data: checkData{
				TotalBlocks:     report.TotalBlocks,
				Corrupt:         report.Corrupt,
				Missing:         report.Missing,
				SnapshotCorrupt: report.SnapshotCorrupt,
			},
			Hint: hintPtr(strings.Join(hintParts, " ")),
		}); err != nil {
			return err
		}
		return nil
	}

	// Quiet mode: success produces no output (exit code is authoritative).
	if globalQuiet {
		return nil
	}

	// Per cli-design.md "输出格式规范": status line first, blank line,
	// core content, blank line, summary. The verbose block is core content
	// and must come AFTER the status line, not before.
	if report.Missing == 0 && report.Corrupt == 0 && report.SnapshotCorrupt == 0 {
		statusOK("Check")
		fmt.Printf("%d blocks passed.\n", report.TotalBlocks)
		if checkVerbose {
			fmt.Println()
			for _, r := range report.VerboseRefs {
				fmt.Printf("  %s:%s  chunk %d  %s\n", r.SnapID, r.FilePath, r.Idx, r.Status)
			}
		}
		unreachable, err := porcelain.CountUnreachableSnapshots(ctx, store, cwd)
		if err != nil {
			slog.Warn("count unreachable snapshots failed", "error", err)
		} else if unreachable > 0 {
			fmt.Printf("  hint: %d unreachable snapshots detected. use 'drift gc --dry-run' to review.\n", unreachable)
		}
		return nil
	}

	statusWarn("Check")
	if checkVerbose {
		fmt.Println()
		for _, r := range report.VerboseRefs {
			fmt.Printf("  %s:%s  chunk %d  %s\n", r.SnapID, r.FilePath, r.Idx, r.Status)
		}
	}
	fmt.Println()
	fmt.Printf("  blocks:  %d total, %d passed\n", report.TotalBlocks, report.TotalBlocks-report.Corrupt-report.Missing)
	fmt.Printf("  corrupt: %d\n", report.Corrupt)
	fmt.Printf("  missing: %d\n", report.Missing)
	if report.SnapshotCorrupt > 0 {
		fmt.Printf("  snapshots: %d corrupt\n", report.SnapshotCorrupt)
	}
	fmt.Println()
	if report.Corrupt > 0 {
		fmt.Println("  hint: corrupt chunks cannot be auto-repaired. Restore affected files from a known-good snapshot using 'drift restore <version>'.")
	}
	if report.Missing > 0 {
		fmt.Println("  hint: missing chunks indicate data loss. Restore from a known-good snapshot using 'drift restore <version>'.")
	}
	if report.SnapshotCorrupt > 0 {
		fmt.Println("  hint: corrupt snapshots have damaged metadata. Use 'drift gc' to clean up unreachable snapshots.")
	}
	return nil
}

// checkData is the JSON data payload of `drift check` on success.
type checkData struct {
	TotalBlocks     int `json:"total_blocks"`
	Corrupt         int `json:"corrupt"`
	Missing         int `json:"missing"`
	SnapshotCorrupt int `json:"snapshot_corrupt"`
}
