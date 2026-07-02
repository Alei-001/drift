package cmd

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/your-org/drift/core"
)

// TestSortSnapshotsNewestFirst_Performance is a regression test for the
// O(N²) sort. With 5000 snapshots sharing the same timestamp (forcing the
// PrevID-chain depth computation), the old code scanned the list for each
// hop. The map-based fix should complete well under 100 ms.
func TestSortSnapshotsNewestFirst_Performance(t *testing.T) {
	const n = 5000
	snaps := make([]*core.Snapshot, n)
	ts := time.Now().Unix()

	// Build a linear chain: snap[i].PrevID = snap[i-1].ID.
	// All same timestamp to force the PrevID depth tiebreaker.
	for i := 0; i < n; i++ {
		var id core.Hash
		binary.BigEndian.PutUint32(id[:4], uint32(i+1)) // +1 to avoid zero hash
		snap := &core.Snapshot{
			ID:        core.SnapshotID{Hash: id},
			Timestamp: ts,
		}
		if i > 0 {
			var prevID core.Hash
			binary.BigEndian.PutUint32(prevID[:4], uint32(i))
			snap.PrevID = &core.SnapshotID{Hash: prevID}
		}
		snaps[i] = snap
	}

	// Reverse so the sort must rearrange (oldest-first → newest-first).
	for i, j := 0, len(snaps)-1; i < j; i, j = i+1, j-1 {
		snaps[i], snaps[j] = snaps[j], snaps[i]
	}

	start := time.Now()
	sortSnapshotsNewestFirst(snaps)
	elapsed := time.Since(start)

	if elapsed >= 100*time.Millisecond {
		t.Fatalf("sorting %d snapshots took %v, want < 100ms", n, elapsed)
	}

	// Verify correctness: the tip (index n, depth n-1) should be first,
	// the root (index 1, depth 0) should be last.
	firstIdx := binary.BigEndian.Uint32(snaps[0].ID.Hash[:4])
	if firstIdx != uint32(n) {
		t.Errorf("expected first snapshot index %d, got %d", n, firstIdx)
	}
	lastIdx := binary.BigEndian.Uint32(snaps[n-1].ID.Hash[:4])
	if lastIdx != 1 {
		t.Errorf("expected last snapshot index 1, got %d", lastIdx)
	}
}
