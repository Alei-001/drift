package porcelain

import (
	"context"
	"strings"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

// countSnapshotDiff returns the number of files that differ (added, removed,
// or content-changed) between two snapshots. Either snapshot may be nil.
func countSnapshotDiff(from, to *core.Snapshot) int {
	if from == nil && to == nil {
		return 0
	}
	if from == nil {
		return len(to.Files)
	}
	if to == nil {
		return len(from.Files)
	}
	fromFiles := make(map[string]core.FileEntry)
	for _, f := range from.Files {
		fromFiles[f.Path] = f
	}
	count := 0
	seen := make(map[string]bool)
	for _, f := range to.Files {
		seen[f.Path] = true
		if prev, ok := fromFiles[f.Path]; !ok {
			count++
		} else if prev.Hash != f.Hash {
			count++
		}
	}
	for p := range fromFiles {
		if !seen[p] {
			count++
		}
	}
	return count
}

// ResolveSnapshotBranches assigns each snapshot to the branch whose tip is
// the nearest descendant (fewest PrevID hops). A snapshot unreachable from
// any branch tip gets no entry. Ties at equal distance are broken by branch
// name for determinism.
func ResolveSnapshotBranches(ctx context.Context, store storage.Storer) (map[string][]string, error) {
	branches, _, err := ListBranches(ctx, store)
	if err != nil {
		return nil, err
	}

	type branchWalk struct {
		name string
		dist map[string]int
	}
	var walks []branchWalk
	for _, b := range branches {
		if b.Target.IsZero() {
			continue
		}
		name := strings.TrimPrefix(b.Name, "heads/")
		bw := branchWalk{name: name, dist: make(map[string]int)}
		currHash := b.Target
		hops := 0
		for !currHash.IsZero() {
			hashStr := currHash.String()
			if _, seen := bw.dist[hashStr]; seen {
				break
			}
			bw.dist[hashStr] = hops
			snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: currHash})
			if err != nil {
				break
			}
			if snap.PrevID == nil {
				break
			}
			currHash = snap.PrevID.Hash
			hops++
		}
		walks = append(walks, bw)
	}

	bestDist := make(map[string]int)
	bestName := make(map[string]string)
	for _, bw := range walks {
		for hashStr, d := range bw.dist {
			cur, ok := bestDist[hashStr]
			if !ok || d < cur || (d == cur && bw.name < bestName[hashStr]) {
				bestDist[hashStr] = d
				bestName[hashStr] = bw.name
			}
		}
	}
	result := make(map[string][]string)
	for hashStr, name := range bestName {
		result[hashStr] = []string{name}
	}
	return result, nil
}
