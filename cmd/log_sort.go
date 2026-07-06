package cmd

import (
	"sort"

	"github.com/your-org/drift/internal/core"
)

// sortSnapshotSummariesNewestFirst sorts snapshot summaries newest-first.
// Primary sort key is timestamp (descending). When timestamps are equal (rapid
// successive saves), it uses the PrevID chain: if A.PrevID == B.ID then A is
// newer than B. This is stable for unrelated summaries.
func sortSnapshotSummariesNewestFirst(snaps []*core.SnapshotSummary) {
	summaryByID := make(map[core.SnapshotID]*core.SnapshotSummary, len(snaps))
	for _, s := range snaps {
		summaryByID[s.ID] = s
	}

	depth := make(map[core.SnapshotID]int, len(snaps))
	for _, start := range snaps {
		if _, ok := depth[start.ID]; ok {
			continue
		}
		var chain []*core.SnapshotSummary
		cur := start
		for cur != nil {
			if d, ok := depth[cur.ID]; ok {
				for i := len(chain) - 1; i >= 0; i-- {
					d++
					depth[chain[i].ID] = d
				}
				chain = nil
				break
			}
			chain = append(chain, cur)
			if cur.PrevID != nil {
				cur = summaryByID[*cur.PrevID]
			} else {
				cur = nil
			}
		}
		for i := len(chain) - 1; i >= 0; i-- {
			depth[chain[i].ID] = len(chain) - 1 - i
		}
	}

	sort.SliceStable(snaps, func(i, j int) bool {
		if snaps[i].Timestamp != snaps[j].Timestamp {
			return snaps[i].Timestamp > snaps[j].Timestamp
		}
		return depth[snaps[i].ID] > depth[snaps[j].ID]
	})
}
