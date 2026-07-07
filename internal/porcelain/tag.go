package porcelain

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/refname"
)

// TagInfo is the user-facing representation of a tag, augmented with the
// target snapshot's message and timestamp so callers can display a tag list
// without a second round-trip per tag.
type TagInfo struct {
	Name    string
	Target  core.SnapshotID
	Message string
	Time    time.Time
}

// ListTags returns all tags sorted by name. Each TagInfo is enriched with
// the target snapshot's message and timestamp when the snapshot is still
// reachable; dangling tags (whose snapshot has been gc'd) get an empty
// message and zero time.
func ListTags(ctx context.Context, store storage.Storer) ([]TagInfo, error) {
	refs, err := store.ListRefs(ctx, "tags/")
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	var tags []TagInfo
	for _, ref := range refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(ref.Name, "tags/")
		info := TagInfo{
			Name:   name,
			Target: core.SnapshotID{Hash: ref.Target},
		}
		if !ref.Target.IsZero() {
			if snap, snapErr := store.GetSnapshot(ctx, core.SnapshotID{Hash: ref.Target}); snapErr == nil {
				info.Message = snap.Message
				info.Time = time.Unix(snap.Timestamp, 0)
			}
		}
		tags = append(tags, info)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Name < tags[j].Name
	})
	return tags, nil
}

// AddTag creates a tag pointing to the given snapshot. It rejects empty
// names, invalid names, zero-hash targets, unknown snapshots, and names
// that already exist. The existence check and the ref write are guarded by
// the workspace lock so that two concurrent AddTag calls for the same name
// cannot both pass the check (TOCTOU).
func AddTag(ctx context.Context, store storage.Storer, cwd string, name string, snapID core.SnapshotID) error {
	if snapID.Hash.IsZero() {
		return fmt.Errorf("cannot create tag pointing to zero hash")
	}
	name = normalizeTagName(name)
	if name == "" {
		return fmt.Errorf("tag name is required")
	}
	if err := refname.Validate("tags/" + name); err != nil {
		return fmt.Errorf("invalid tag name: %w", err)
	}

	if err := AcquireWorkspaceLock(cwd); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(cwd)

	if _, err := store.GetSnapshot(ctx, snapID); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("snapshot '%s' not found: %w", snapID.Hash.String(), ErrSnapshotNotFound)
		}
		return fmt.Errorf("check snapshot existence: %w", err)
	}

	refName := "tags/" + name
	if existing, err := store.GetRef(ctx, refName); err == nil && existing != nil {
		return fmt.Errorf("tag '%s' already exists: %w", name, ErrTagAlreadyExists)
	} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("check tag existence: %w", err)
	}

	ref := &core.Reference{
		Type:   core.RefTypeTag,
		Name:   refName,
		Target: snapID.Hash,
	}
	if err := store.SetRef(ctx, refName, ref); err != nil {
		return fmt.Errorf("set tag ref: %w", err)
	}
	return nil
}

// DeleteTag removes a tag by name. It returns ErrTagNotFound if the tag
// does not exist. The existence check and the ref delete are guarded by the
// workspace lock so that two concurrent DeleteTag calls cannot race on the
// same name (TOCTOU).
func DeleteTag(ctx context.Context, store storage.Storer, cwd string, name string) error {
	name = normalizeTagName(name)
	if name == "" {
		return fmt.Errorf("tag name is required")
	}
	if err := refname.Validate("tags/" + name); err != nil {
		return fmt.Errorf("invalid tag name: %w", err)
	}

	if err := AcquireWorkspaceLock(cwd); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(cwd)

	refName := "tags/" + name
	if _, err := store.GetRef(ctx, refName); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("tag '%s' not found: %w", name, ErrTagNotFound)
		}
		return fmt.Errorf("check tag existence: %w", err)
	}
	if err := store.DeleteRef(ctx, refName); err != nil {
		return fmt.Errorf("delete tag ref: %w", err)
	}
	return nil
}

// RenameTag renames an existing tag. The new name is NFC-normalized and
// validated before use. The operation is ordered as SetRef(new) then
// DeleteRef(old) so a crash leaves a duplicate rather than a missing tag,
// mirroring RenameBranch's safety property. The existence checks and ref
// writes are guarded by the workspace lock so that concurrent RenameTag
// calls cannot race on the same names (TOCTOU).
func RenameTag(ctx context.Context, store storage.Storer, cwd string, oldName, newName string) error {
	oldName = normalizeTagName(oldName)
	newName = normalizeTagName(newName)
	if oldName == "" {
		return fmt.Errorf("old tag name is required")
	}
	if err := refname.Validate("tags/" + oldName); err != nil {
		return fmt.Errorf("invalid old tag name: %w", err)
	}
	if newName == "" {
		return fmt.Errorf("new tag name is required")
	}
	if err := refname.Validate("tags/" + newName); err != nil {
		return fmt.Errorf("invalid tag name: %w", err)
	}

	if err := AcquireWorkspaceLock(cwd); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(cwd)

	oldRefName := "tags/" + oldName
	newRefName := "tags/" + newName

	oldRef, err := store.GetRef(ctx, oldRefName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("tag '%s' not found: %w", oldName, ErrTagNotFound)
		}
		return fmt.Errorf("check tag existence: %w", err)
	}
	if oldName == newName {
		return nil
	}
	if _, err := store.GetRef(ctx, newRefName); err == nil {
		return fmt.Errorf("tag '%s' already exists: %w", newName, ErrTagAlreadyExists)
	} else if !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("check tag existence: %w", err)
	}

	newRef := &core.Reference{
		Type:   core.RefTypeTag,
		Name:   newRefName,
		Target: oldRef.Target,
	}
	if err := store.SetRef(ctx, newRefName, newRef); err != nil {
		return fmt.Errorf("create renamed tag: %w", err)
	}
	if err := store.DeleteRef(ctx, oldRefName); err != nil {
		return fmt.Errorf("remove old tag: %w", err)
	}
	return nil
}

// normalizeTagName applies Unicode NFC normalization to a tag name so that
// visually identical names with different code-point sequences (e.g.
// U+00E9 vs U+0065 U+0301) collapse to a single stored form. This prevents
// accidental duplicate tags that look the same but are stored differently.
func normalizeTagName(name string) string {
	return norm.NFC.String(name)
}
