package porcelain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/your-org/drift/internal/core"
)

// TestSaveTag_Success verifies the happy path: a new tag is created with the
// correct type, name, and target, and can be read back from the store.
func TestSaveTag_Success(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	var snapHash core.Hash
	snapHash[0] = 0xBB

	if err := SaveTag(context.Background(), store, dir, "v1.0", snapHash); err != nil {
		t.Fatalf("SaveTag failed: %v", err)
	}

	ref, err := store.GetRef(context.Background(), "tags/v1.0")
	if err != nil {
		t.Fatalf("GetRef tags/v1.0: %v", err)
	}
	if ref.Type != core.RefTypeTag {
		t.Errorf("expected RefTypeTag, got %v", ref.Type)
	}
	if ref.Name != "tags/v1.0" {
		t.Errorf("expected name 'tags/v1.0', got %q", ref.Name)
	}
	if ref.Target != snapHash {
		t.Errorf("expected target %x, got %x", snapHash, ref.Target)
	}
}

// TestSaveTag_ZeroHashRejected verifies that creating a tag pointing at the
// zero hash is rejected. The zero hash represents "no snapshot", so allowing
// it would create a dangling tag.
func TestSaveTag_ZeroHashRejected(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	err := SaveTag(context.Background(), store, dir, "v1.0", core.Hash{})
	if err == nil {
		t.Fatal("expected error for zero hash, got nil")
	}
	if !strings.Contains(err.Error(), "zero hash") {
		t.Errorf("expected 'zero hash' in error, got %q", err.Error())
	}

	// No ref should have been written.
	if _, err := store.GetRef(context.Background(), "tags/v1.0"); err == nil {
		t.Error("expected tags/v1.0 to not exist after failed SaveTag")
	}
}

// TestSaveTag_EmptyNameRejected verifies that an empty tag name is rejected
// before any storage mutation.
func TestSaveTag_EmptyNameRejected(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	var snapHash core.Hash
	snapHash[0] = 0xBB

	err := SaveTag(context.Background(), store, dir, "", snapHash)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
	if !strings.Contains(err.Error(), "tag name is required") {
		t.Errorf("expected 'tag name is required' in error, got %q", err.Error())
	}
}

// TestSaveTag_InvalidNameRejected verifies that names failing refname.Validate
// are rejected. The validation runs on "tags/" + name, so the rules apply to
// the full "tags/<name>" string: "..", ":", spaces, control chars, and
// Windows-reserved basenames are all rejected.
func TestSaveTag_InvalidNameRejected(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	var snapHash core.Hash
	snapHash[0] = 0xBB

	invalid := []string{
		"v1..2",        // contains '..'
		"with:colon",   // contains ':'
		"with space",   // contains space
		"CON",          // windows-reserved basename (tags/CON -> base "con")
	}
	for _, name := range invalid {
		t.Run(name, func(t *testing.T) {
			err := SaveTag(context.Background(), store, dir, name, snapHash)
			if err == nil {
				t.Errorf("expected error for invalid name %q, got nil", name)
			}
		})
	}
}

// TestSaveTag_DoesNotMutateStoreOnFailure verifies that a failed SaveTag
// (zero hash or invalid name) does not leave a half-written tag ref behind.
func TestSaveTag_DoesNotMutateStoreOnFailure(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	// Both should fail and leave no trace.
	_ = SaveTag(context.Background(), store, dir, "v1.0", core.Hash{})
	_ = SaveTag(context.Background(), store, dir, "v1..bad", core.Hash{0x01})

	refs, err := store.ListRefs(context.Background(), "tags/")
	if err != nil {
		t.Fatalf("ListRefs tags/: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected no tags after failed SaveTag calls, got %d: %v", len(refs), refs)
	}
}

// putTestSnapshot stores a minimal snapshot in the memory backend for tag
// tests that need a valid snapshot ID to point at.
func putTestSnapshot(t *testing.T, store interface{ PutSnapshot(context.Context, *core.Snapshot) error }) core.SnapshotID {
	t.Helper()
	snap := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{0xAA, 0xBB}},
		Message:   "test snapshot",
		Timestamp: 1700000000,
	}
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
