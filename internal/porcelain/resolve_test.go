package porcelain

import (
	"context"
	"errors"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

// TestResolveHeadSnapshot_BranchRef follows HEAD -> heads/main -> snapshot
// (the symbolic-ref path) and returns the snapshot the branch points at.
// The existing diff_test.go covers the detached-HEAD path; this covers symref.
func TestResolveHeadSnapshot_BranchRef(t *testing.T) {
	store := memory.NewMemoryStorage()
	snapHash := gcPutSnapshot(store, 0x01, nil, nil)

	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: snapHash,
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})

	snap := ResolveHeadSnapshot(context.Background(), store)
	if snap == nil {
		t.Fatal("expected non-nil snapshot, got nil")
	}
	if snap.ID.Hash != snapHash {
		t.Errorf("expected snapshot hash %x, got %x", snapHash, snap.ID.Hash)
	}
}

// TestResolveHeadSnapshot_BranchRefMissingSnapshot returns nil when HEAD
// is a symref pointing at a branch whose target snapshot is missing from
// the store.
func TestResolveHeadSnapshot_BranchRefMissingSnapshot(t *testing.T) {
	store := memory.NewMemoryStorage()
	missingHash := gcHash(0xff)
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: missingHash,
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})

	snap := ResolveHeadSnapshot(context.Background(), store)
	if snap != nil {
		t.Errorf("expected nil when branch target missing, got %v", snap)
	}
}

// TestResolveSnapshotRef_HeadKeyword verifies that the "head" keyword
// resolves to the current HEAD snapshot via ResolveSnapshotRef.
func TestResolveSnapshotRef_HeadKeyword(t *testing.T) {
	store := memory.NewMemoryStorage()
	snapHash := gcPutSnapshot(store, 0x04, nil, nil)

	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: snapHash,
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})

	snap, err := ResolveSnapshotRef(context.Background(), store, "head")
	if err != nil {
		t.Fatalf("ResolveSnapshotRef head: %v", err)
	}
	if snap.ID.Hash != snapHash {
		t.Errorf("expected hash %x, got %x", snapHash, snap.ID.Hash)
	}
}

// TestResolveSnapshotRef_HeadNotFound verifies that resolving "head" when
// HEAD does not exist returns ErrNotARepo (the workspace has not been
// initialized). This distinguishes "no repo" from "repo exists but has no
// commits yet" (which would be ErrSnapshotNotFound) so `drift log` exits
// with ExitNotRepo in a non-repo directory.
func TestResolveSnapshotRef_HeadNotFound(t *testing.T) {
	store := memory.NewMemoryStorage()

	_, err := ResolveSnapshotRef(context.Background(), store, "head")
	if err == nil {
		t.Fatal("expected error resolving head with no HEAD ref, got nil")
	}
	if !errors.Is(err, ErrNotARepo) {
		t.Errorf("expected ErrNotARepo, got %v", err)
	}
}

// TestResolveSnapshotRef_BranchSyntax verifies that branch:<name> resolves
// via the heads/<name> reference.
func TestResolveSnapshotRef_BranchSyntax(t *testing.T) {
	store := memory.NewMemoryStorage()
	snapHash := gcPutSnapshot(store, 0x05, nil, nil)

	store.SetRef(context.Background(), "heads/feature", &core.Reference{
		Name:   "heads/feature",
		Type:   core.RefTypeBranch,
		Target: snapHash,
	})

	snap, err := ResolveSnapshotRef(context.Background(), store, "branch:feature")
	if err != nil {
		t.Fatalf("ResolveSnapshotRef branch:feature: %v", err)
	}
	if snap.ID.Hash != snapHash {
		t.Errorf("expected hash %x, got %x", snapHash, snap.ID.Hash)
	}
}

// TestResolveSnapshotRef_BranchMissing verifies that resolving a branch that
// does not exist returns ErrSnapshotNotFound.
func TestResolveSnapshotRef_BranchMissing(t *testing.T) {
	store := memory.NewMemoryStorage()

	_, err := ResolveSnapshotRef(context.Background(), store, "branch:nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

// TestResolveSnapshotRef_BareNameAsBranch verifies that a bare name (without
// the branch: prefix) is treated as branch:<name>.
func TestResolveSnapshotRef_BareNameAsBranch(t *testing.T) {
	store := memory.NewMemoryStorage()
	snapHash := gcPutSnapshot(store, 0x06, nil, nil)

	store.SetRef(context.Background(), "heads/dev", &core.Reference{
		Name:   "heads/dev",
		Type:   core.RefTypeBranch,
		Target: snapHash,
	})

	snap, err := ResolveSnapshotRef(context.Background(), store, "dev")
	if err != nil {
		t.Fatalf("ResolveSnapshotRef dev: %v", err)
	}
	if snap.ID.Hash != snapHash {
		t.Errorf("expected hash %x, got %x", snapHash, snap.ID.Hash)
	}
}

// TestResolveSnapshotRef_IDPrefixTooShort verifies that an id:<prefix>
// shorter than minHashPrefixLen is rejected.
func TestResolveSnapshotRef_IDPrefixTooShort(t *testing.T) {
	store := memory.NewMemoryStorage()

	_, err := ResolveSnapshotRef(context.Background(), store, "id:abc")
	if err == nil {
		t.Fatal("expected error for short prefix, got nil")
	}
}

// TestResolveSnapshotRef_IDPrefixNotFound verifies that an id:<prefix> that
// matches no snapshot returns ErrSnapshotNotFound.
func TestResolveSnapshotRef_IDPrefixNotFound(t *testing.T) {
	store := memory.NewMemoryStorage()
	gcPutSnapshot(store, 0x07, nil, nil)

	_, err := ResolveSnapshotRef(context.Background(), store, "id:00000000")
	if err == nil {
		t.Fatal("expected error for non-matching prefix, got nil")
	}
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

// TestResolveSnapshotRef_IDPrefixMatch verifies that an id:<prefix> that
// matches exactly one snapshot returns that snapshot.
func TestResolveSnapshotRef_IDPrefixMatch(t *testing.T) {
	store := memory.NewMemoryStorage()
	snapHash := gcPutSnapshot(store, 0x08, nil, nil)

	prefix := snapHash.FullString()[:8]
	snap, err := ResolveSnapshotRef(context.Background(), store, "id:"+prefix)
	if err != nil {
		t.Fatalf("ResolveSnapshotRef id:%s: %v", prefix, err)
	}
	if snap.ID.Hash != snapHash {
		t.Errorf("expected hash %x, got %x", snapHash, snap.ID.Hash)
	}
}

// TestResolveSnapshotRef_TagSyntax verifies that tag:<name> resolves via
// the tags/<name> reference.
func TestResolveSnapshotRef_TagSyntax(t *testing.T) {
	store := memory.NewMemoryStorage()
	snapHash := gcPutSnapshot(store, 0x09, nil, nil)

	store.SetRef(context.Background(), "tags/v1", &core.Reference{
		Name:   "tags/v1",
		Type:   core.RefTypeTag,
		Target: snapHash,
	})

	snap, err := ResolveSnapshotRef(context.Background(), store, "tag:v1")
	if err != nil {
		t.Fatalf("ResolveSnapshotRef tag:v1: %v", err)
	}
	if snap.ID.Hash != snapHash {
		t.Errorf("expected hash %x, got %x", snapHash, snap.ID.Hash)
	}
}

// TestResolveSnapshotRef_TagMissing verifies that resolving a tag that does
// not exist returns ErrSnapshotNotFound.
func TestResolveSnapshotRef_TagMissing(t *testing.T) {
	store := memory.NewMemoryStorage()

	_, err := ResolveSnapshotRef(context.Background(), store, "tag:nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent tag, got nil")
	}
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
}

// TestResolveSnapshotRef_IDPrefixAmbiguous verifies that an id:<prefix>
// matching more than one snapshot returns an error wrapping ErrAmbiguousID.
func TestResolveSnapshotRef_IDPrefixAmbiguous(t *testing.T) {
	store := memory.NewMemoryStorage()
	hash1 := gcPutSnapshot(store, 0x0a, nil, nil)
	hash2 := gcPutSnapshot(store, 0x0b, nil, nil)

	h1 := hash1.FullString()
	h2 := hash2.FullString()

	// Find common prefix length.
	commonLen := 0
	for i := 0; i < len(h1) && i < len(h2); i++ {
		if h1[i] != h2[i] {
			break
		}
		commonLen++
	}

	if commonLen < minHashPrefixLen {
		t.Skip("hashes share no common prefix >= minHashPrefixLen; cannot test ambiguity")
	}

	prefix := h1[:commonLen]
	_, err := ResolveSnapshotRef(context.Background(), store, "id:"+prefix)
	if err == nil {
		t.Fatal("expected ambiguous error, got nil")
	}
	if !errors.Is(err, ErrAmbiguousID) {
		t.Errorf("expected ErrAmbiguousID, got %v", err)
	}
}
