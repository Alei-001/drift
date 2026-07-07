package porcelain

import (
	"context"
	"sort"
	"strings"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

// ResolveBranchTips returns a map from snapshot hash to the list of branch
// names whose tip (Target) points directly at that snapshot. A snapshot that
// is not the tip of any branch gets no entry.
//
// Unlike ResolveSnapshotBranches (which attributes every reachable snapshot to
// its nearest branch tip), this only marks snapshots that ARE branch tips.
// This mirrors git's --decorate=short behavior: the branch column in 'log'
// shows where each branch head sits, leaving the rest of the chain unlabeled
// so the user can see at a glance where branches diverge.
//
// The returned branch names are sorted alphabetically for stable display.
func ResolveBranchTips(ctx context.Context, store storage.Storer) (map[string][]string, error) {
	branches, _, err := ListBranches(ctx, store)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for _, b := range branches {
		if b.Target.IsZero() {
			continue
		}
		name := strings.TrimPrefix(b.Name, "heads/")
		hashStr := b.Target.String()
		result[hashStr] = append(result[hashStr], name)
	}
	for hashStr := range result {
		sort.Strings(result[hashStr])
	}
	return result, nil
}

// WalkSnapshotChain walks the PrevID chain starting from startHash and returns
// snapshot summaries in chain order (newest first). The walk stops at the
// first missing snapshot, a nil PrevID, or a zero PrevID hash. It is
// context-cancellable.
//
// This is the core of 'drift log' default and --branch modes: by walking only
// the current branch's chain, inherited commits from parent branches are
// included (matching git log semantics), giving the user the full evolution
// history of the branch.
func WalkSnapshotChain(ctx context.Context, store storage.Storer, startHash core.Hash) ([]*core.SnapshotSummary, error) {
	var summaries []*core.SnapshotSummary
	currHash := startHash
	for !currHash.IsZero() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: currHash})
		if err != nil {
			break
		}
		summaries = append(summaries, snapshotToSummary(snap))
		if snap.PrevID == nil {
			break
		}
		currHash = snap.PrevID.Hash
	}
	return summaries, nil
}

// snapshotToSummary converts a full Snapshot to its lightweight Summary form.
// Tags are copied defensively so callers cannot mutate the snapshot's slice.
func snapshotToSummary(s *core.Snapshot) *core.SnapshotSummary {
	ss := &core.SnapshotSummary{
		ID:        s.ID,
		Message:   s.Message,
		Author:    s.Author,
		Timestamp: s.Timestamp,
		TotalSize: s.TotalSize,
	}
	if s.PrevID != nil {
		prev := *s.PrevID
		ss.PrevID = &prev
	}
	if len(s.Tags) > 0 {
		ss.Tags = append([]string(nil), s.Tags...)
	}
	return ss
}

// ResolveCurrentBranchName returns the name of the current branch (without the
// "heads/" prefix), or "" if HEAD is detached or unreadable.
func ResolveCurrentBranchName(ctx context.Context, store storage.Storer) string {
	headRef, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		return ""
	}
	if headRef.SymRef == "" {
		return ""
	}
	return strings.TrimPrefix(headRef.SymRef, "heads/")
}
