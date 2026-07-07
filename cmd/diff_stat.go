package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/format"
)

// diffStatSnapshots prints a --stat summary between two snapshots. The
// per-file computation lives in porcelain; this function only renders.
func diffStatSnapshots(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot) error {
	stats, err := porcelain.ComputeStatSnapshots(ctx, store, snap1, snap2)
	if err != nil {
		return err
	}
	printStatOutput(stats)
	return nil
}

// diffStatWorkspace prints a --stat summary between workspace and snapshot.
// The per-file computation lives in porcelain; this function only renders.
func diffStatWorkspace(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot) error {
	stats, err := porcelain.ComputeStatWorkspace(ctx, store, cwd, cfg, snap)
	if err != nil {
		return err
	}
	printStatOutput(stats)
	return nil
}

// maxStatBars caps the visual length of the +/- bar in --stat output.
const maxStatBars = 10

// statBar renders a compact visual bar of insertions (+) and deletions (-),
// scaled to at most maxStatBars characters.
func statBar(ins, del int) string {
	total := ins + del
	if total == 0 {
		return ""
	}
	bars := maxStatBars
	if total < bars {
		bars = total
	}
	plus := bars * ins / total
	return strings.Repeat("+", plus) + strings.Repeat("-", bars-plus)
}

// printStatOutput prints the --stat file list and summary line.
func printStatOutput(stats []porcelain.FileStat) {
	if len(stats) == 0 {
		fmt.Println()
		fmt.Println("  No changes.")
		return
	}
	fmt.Println()
	pathWidth := 0
	for _, s := range stats {
		if len(s.Path) > pathWidth {
			pathWidth = len(s.Path)
		}
	}
	totalIns, totalDel := 0, 0
	for _, s := range stats {
		if s.Binary {
			fmt.Printf("  %-*s | Bin %s -> %s\n", pathWidth, s.Path,
				format.Bytes(s.OldSize), format.Bytes(s.NewSize))
			continue
		}
		total := s.Insertions + s.Deletions
		fmt.Printf("  %-*s | %d %s\n", pathWidth, s.Path, total, statBar(s.Insertions, s.Deletions))
		totalIns += s.Insertions
		totalDel += s.Deletions
	}
	fmt.Printf("\n  %d %s changed, %d insertions(+), %d deletions(-)\n",
		len(stats), pluralFile(len(stats)), totalIns, totalDel)
}
