package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/your-org/drift/internal/porcelain"
)

var (
	gcDryRun   bool
	gcKeepAuto int
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Reclaim unreachable snapshots and chunks",
	Long:  "Reclaim snapshots and chunks no longer reachable from any branch or tag reference, freeing storage space.",
	RunE:  runGC,
}

func init() {
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "preview only, do not delete")
	gcCmd.Flags().IntVar(&gcKeepAuto, "keep-auto", 0, "keep the N most recent unreachable auto-saves")
	rootCmd.AddCommand(gcCmd)
}

func runGC(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cwd, err := getCwd(cmd)
	if err != nil {
		return err
	}
	store, _, err := openProjectOrReport("GC", "gc", cwd)
	if err != nil {
		return err
	}
	defer store.Close()

	report, err := porcelain.CollectGarbage(ctx, store, cwd, gcDryRun, gcKeepAuto)
	if err != nil {
		reportFailed("GC", "gc", err.Error(), "")
		return ErrSilent
	}

	if globalJSON {
		if err := outputJSON(JSONEnvelope{
			Command: "gc",
			Status:  "ok",
			Data: gcData{
				DryRun:           gcDryRun,
				SnapshotsRemoved: report.SnapshotsRemoved,
				ChunksRemoved:    report.ChunksRemoved,
				FreedBytes:       report.FreedBytes,
				AutoKept:         report.AutoKept,
			},
		}); err != nil {
			return err
		}
		return nil
	}

	// Quiet mode: success produces no output (exit code is authoritative).
	if globalQuiet {
		return nil
	}

	if report.SnapshotsRemoved == 0 && report.ChunksRemoved == 0 {
		statusOK("GC")
		fmt.Println("  nothing to reclaim.")
		return nil
	}

	verb := "removed"
	if gcDryRun {
		verb = "would be removed"
	}

	if gcDryRun {
		fmt.Println(">>> GC [dry-run]")
	} else {
		statusOK("GC")
	}

	snapshotLine := fmt.Sprintf("  snapshots:  %d %s", report.SnapshotsRemoved, verb)
	if report.AutoKept > 0 {
		snapshotLine += fmt.Sprintf(" (%d auto-saves kept by --keep-auto=%d)", report.AutoKept, gcKeepAuto)
	}
	fmt.Println(snapshotLine)
	fmt.Printf("  chunks:     %d %s\n", report.ChunksRemoved, verb)
	if gcDryRun {
		fmt.Printf("  freed:      ~%s\n", formatSize(report.FreedBytes))
	} else {
		fmt.Printf("  freed:      %s\n", formatSize(report.FreedBytes))
	}
	return nil
}

// gcData is the JSON data payload of `drift gc` on success.
type gcData struct {
	DryRun           bool  `json:"dry_run"`
	SnapshotsRemoved int   `json:"snapshots_removed"`
	ChunksRemoved    int   `json:"chunks_removed"`
	FreedBytes       int64 `json:"freed_bytes"`
	AutoKept         int   `json:"auto_kept"`
}
