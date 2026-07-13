package porcelain

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// putTestSnapshot stores a minimal snapshot in the memory backend for tag
// tests that need a valid snapshot ID to point at. The ID is computed from
// the snapshot content (BLAKE3 of marshaled proto with IdHash omitted) so
// that GetSnapshot's integrity check passes when AddTag verifies existence.
func putTestSnapshot(t *testing.T, store interface {
	PutSnapshot(context.Context, *core.Snapshot) error
}) core.SnapshotID {
	t.Helper()
	snap := &core.Snapshot{
		Message:   "test snapshot",
		Timestamp: 1700000000,
	}
	p := core.SnapshotToProto(snap, false)
	marshaled, _ := proto.Marshal(p)
	snap.ID = core.SnapshotID{Hash: core.Hash(blake3.Sum256(marshaled))}
	if err := store.PutSnapshot(context.Background(), snap); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}
	return snap.ID
}

// TestListTags_SortedWithSnapshotInfo verifies ListTags returns tags sorted
// by name and enriched with the target snapshot's message and timestamp.
func TestListTags_SortedWithSnapshotInfo(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	if err := AddTag(context.Background(), store, dir, "v2", snapID); err != nil {
		t.Fatalf("AddTag v2: %v", err)
	}
	if err := AddTag(context.Background(), store, dir, "v1", snapID); err != nil {
		t.Fatalf("AddTag v1: %v", err)
	}

	tags, err := ListTags(context.Background(), store)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].Name != "v1" || tags[1].Name != "v2" {
		t.Errorf("expected [v1, v2], got [%s, %s]", tags[0].Name, tags[1].Name)
	}
	if tags[0].Target != snapID {
		t.Errorf("expected target %x, got %x", snapID.Hash, tags[0].Target.Hash)
	}
	if tags[0].Message != "test snapshot" {
		t.Errorf("expected message 'test snapshot', got %q", tags[0].Message)
	}
	if tags[0].Time.IsZero() {
		t.Error("expected non-zero time")
	}
}

// TestListTags_Empty verifies ListTags returns an empty slice when no tags
// exist.
func TestListTags_Empty(t *testing.T) {
	store := setupTestStore(t)
	tags, err := ListTags(context.Background(), store)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

// TestAddTag_Success creates a tag and verifies it can be read back.
func TestAddTag_Success(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	if err := AddTag(context.Background(), store, dir, "v1", snapID); err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	ref, err := store.GetRef(context.Background(), "tags/v1")
	if err != nil {
		t.Fatalf("GetRef: %v", err)
	}
	if ref.Target != snapID.Hash {
		t.Errorf("expected target %x, got %x", snapID.Hash, ref.Target)
	}
}

// TestAddTag_AlreadyExists verifies that adding a duplicate tag returns
// ErrTagAlreadyExists.
func TestAddTag_AlreadyExists(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	if err := AddTag(context.Background(), store, dir, "v1", snapID); err != nil {
		t.Fatalf("first AddTag: %v", err)
	}
	err := AddTag(context.Background(), store, dir, "v1", snapID)
	if !errors.Is(err, ErrTagAlreadyExists) {
		t.Fatalf("expected ErrTagAlreadyExists, got %v", err)
	}
}

// TestAddTag_SnapshotNotFound verifies that adding a tag pointing to a
// non-existent snapshot returns ErrSnapshotNotFound.
func TestAddTag_SnapshotNotFound(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	missingID := core.SnapshotID{Hash: core.Hash{0x99}}
	err := AddTag(context.Background(), store, dir, "v1", missingID)
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("expected ErrSnapshotNotFound, got %v", err)
	}
}

// TestAddTag_NFCEquivalent verifies that two visually identical names with
// different Unicode code-point sequences (NFC vs NFD) map to the same tag
// after normalization.
func TestAddTag_NFCEquivalent(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	// "é" as a single precomposed code point (U+00E9) — already NFC.
	nfc := "\u00e9"
	// "é" as a decomposed sequence (U+0065 U+0301) — NFD form.
	nfd := "e\u0301"

	if err := AddTag(context.Background(), store, dir, nfc, snapID); err != nil {
		t.Fatalf("AddTag nfc: %v", err)
	}
	// Adding the NFD form should be treated as a duplicate after NFC
	// normalization.
	err := AddTag(context.Background(), store, dir, nfd, snapID)
	if !errors.Is(err, ErrTagAlreadyExists) {
		t.Fatalf("expected ErrTagAlreadyExists for NFD equivalent, got %v", err)
	}
}

// TestDeleteTag_Success creates then deletes a tag.
func TestDeleteTag_Success(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	if err := AddTag(context.Background(), store, dir, "v1", snapID); err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	if err := DeleteTag(context.Background(), store, dir, "v1"); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := store.GetRef(context.Background(), "tags/v1"); err == nil {
		t.Error("expected tag to be deleted")
	}
}

// TestDeleteTag_NotFound verifies that deleting a non-existent tag returns
// ErrTagNotFound.
func TestDeleteTag_NotFound(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	err := DeleteTag(context.Background(), store, dir, "v1")
	if !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("expected ErrTagNotFound, got %v", err)
	}
}

// TestRenameTag_Success renames a tag and verifies the old name is gone and
// the new name points to the same target.
func TestRenameTag_Success(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	if err := AddTag(context.Background(), store, dir, "v1", snapID); err != nil {
		t.Fatalf("AddTag: %v", err)
	}
	if err := RenameTag(context.Background(), store, dir, "v1", "v2"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}
	if _, err := store.GetRef(context.Background(), "tags/v1"); err == nil {
		t.Error("expected old tag to be gone")
	}
	ref, err := store.GetRef(context.Background(), "tags/v2")
	if err != nil {
		t.Fatalf("GetRef tags/v2: %v", err)
	}
	if ref.Target != snapID.Hash {
		t.Errorf("expected target %x, got %x", snapID.Hash, ref.Target)
	}
}

// TestRenameTag_OldNotFound verifies renaming a non-existent tag returns
// ErrTagNotFound.
func TestRenameTag_OldNotFound(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	err := RenameTag(context.Background(), store, dir, "v1", "v2")
	if !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("expected ErrTagNotFound, got %v", err)
	}
}

// TestRenameTag_TargetExists verifies renaming to an existing tag name
// returns ErrTagAlreadyExists.
func TestRenameTag_TargetExists(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	if err := AddTag(context.Background(), store, dir, "v1", snapID); err != nil {
		t.Fatalf("AddTag v1: %v", err)
	}
	if err := AddTag(context.Background(), store, dir, "v2", snapID); err != nil {
		t.Fatalf("AddTag v2: %v", err)
	}
	err := RenameTag(context.Background(), store, dir, "v1", "v2")
	if !errors.Is(err, ErrTagAlreadyExists) {
		t.Fatalf("expected ErrTagAlreadyExists, got %v", err)
	}
}

// TestAddTag_ZeroHashRejected verifies that creating a tag pointing at the
// zero hash is rejected. The zero hash represents "no snapshot", so allowing
// it would create a dangling tag.
func TestAddTag_ZeroHashRejected(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	err := AddTag(context.Background(), store, dir, "v1.0", core.SnapshotID{})
	if err == nil {
		t.Fatal("expected error for zero hash, got nil")
	}
	if !strings.Contains(err.Error(), "zero hash") {
		t.Errorf("expected 'zero hash' in error, got %q", err.Error())
	}

	// No ref should have been written.
	if _, err := store.GetRef(context.Background(), "tags/v1.0"); err == nil {
		t.Error("expected tags/v1.0 to not exist after failed AddTag")
	}
}

// TestAddTag_EmptyNameRejected verifies that an empty tag name is rejected
// before any storage mutation.
func TestAddTag_EmptyNameRejected(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	err := AddTag(context.Background(), store, dir, "", snapID)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if !strings.Contains(err.Error(), "tag name is required") {
		t.Errorf("expected 'tag name is required' in error, got %q", err.Error())
	}
}

// TestAddTag_InvalidNameRejected verifies that names failing refname.Validate
// are rejected. The validation runs on "tags/" + name, so the rules apply to
// the full "tags/<name>" string: "..", ":", spaces, control chars, and
// Windows-reserved basenames are all rejected.
func TestAddTag_InvalidNameRejected(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()
	snapID := putTestSnapshot(t, store)

	invalid := []string{
		"v1..2",      // contains '..'
		"with:colon", // contains ':'
		"with space", // contains space
		"CON",        // windows-reserved basename (tags/CON -> base "con")
	}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			err := AddTag(context.Background(), store, dir, name, snapID)
			if err == nil {
				t.Errorf("expected error for invalid name %q, got nil", name)
			}
		})
	}
}

// TestAddTag_DoesNotMutateStoreOnFailure verifies that a failed AddTag
// (zero hash or invalid name) does not leave a half-written tag ref behind.
func TestAddTag_DoesNotMutateStoreOnFailure(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	// Both should fail and leave no trace. The invalid name uses a non-zero
	// hash so it reaches the name-validation path (not the zero-hash short
	// circuit) but still fails before writing.
	_ = AddTag(context.Background(), store, dir, "v1.0", core.SnapshotID{})
	_ = AddTag(context.Background(), store, dir, "v1..bad", core.SnapshotID{Hash: core.Hash{0x01}})

	refs, err := store.ListRefs(context.Background(), "tags/")
	if err != nil {
		t.Fatalf("ListRefs tags/: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected no tags after failed AddTag calls, got %d: %v", len(refs), refs)
	}
}

// TestAddTag_ConcurrentSameName is a regression test for the TOCTOU race in
// AddTag: two goroutines creating the same tag name simultaneously must not
// both succeed. Before the workspace lock was added, both could pass the
// existence check and the second would silently overwrite the first. With the
// lock, the calls serialize: exactly one succeeds, and the loser returns an
// error (ErrTagAlreadyExists if it ran after the winner released the lock, or
// ErrLocked if it contended on the lock while the winner held it). Either way
// no double-create / overwrite occurs.
func TestAddTag_ConcurrentSameName(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	snap1 := putTestSnapshot(t, store)
	snap2 := &core.Snapshot{
		Message:   "second snapshot",
		Timestamp: 1700000001,
	}
	p2 := core.SnapshotToProto(snap2, false)
	marshaled2, _ := proto.Marshal(p2)
	snap2.ID = core.SnapshotID{Hash: core.Hash(blake3.Sum256(marshaled2))}
	if err := store.PutSnapshot(context.Background(), snap2); err != nil {
		t.Fatalf("PutSnapshot snap2: %v", err)
	}
	snap2ID := snap2.ID

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		err1  error
		err2  error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		err1 = AddTag(context.Background(), store, dir, "v1", snap1)
	}()
	go func() {
		defer wg.Done()
		<-start
		err2 = AddTag(context.Background(), store, dir, "v1", snap2ID)
	}()
	close(start)
	wg.Wait()

	// Exactly one must succeed; both succeeding is the TOCTOU bug.
	successCount := 0
	if err1 == nil {
		successCount++
	}
	if err2 == nil {
		successCount++
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one success, got %d (err1=%v, err2=%v)", successCount, err1, err2)
	}

	// The loser must return an error that signals the tag is taken or the
	// workspace is locked — never a silent overwrite.
	var loserErr error
	if err1 != nil {
		loserErr = err1
	} else {
		loserErr = err2
	}
	if !errors.Is(loserErr, ErrTagAlreadyExists) && !errors.Is(loserErr, ErrLocked) {
		t.Fatalf("expected loser error to be ErrTagAlreadyExists or ErrLocked, got %v", loserErr)
	}

	// The stored tag must point at the winner's snapshot, not be clobbered.
	ref, err := store.GetRef(context.Background(), "tags/v1")
	if err != nil {
		t.Fatalf("GetRef tags/v1: %v", err)
	}
	if ref.Target != snap1.Hash && ref.Target != snap2ID.Hash {
		t.Errorf("tag target %x does not match either snapshot hash", ref.Target)
	}
}
