package porcelain

import (
	"context"
	"errors"
	"fmt"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/refname"
)

// SaveTag creates a tag ref pointing at snapshotID. The existence check and
// the ref write are guarded by the workspace lock so that two concurrent
// SaveTag calls for the same name cannot both pass the check and the second
// cannot silently overwrite the first (TOCTOU).
func SaveTag(ctx context.Context, store storage.Storer, cwd string, name string, snapshotID core.Hash) error {
	if snapshotID.IsZero() {
		return fmt.Errorf("cannot create tag pointing to zero hash")
	}
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
	existing, err := store.GetRef(ctx, refName)
	if err == nil && existing != nil {
		return fmt.Errorf("tag '%s' already exists: %w", name, ErrTagAlreadyExists)
	} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("check tag existence: %w", err)
	}

	ref := &core.Reference{
		Type:   core.RefTypeTag,
		Name:   refName,
		Target: snapshotID,
	}
	if err := store.SetRef(ctx, refName, ref); err != nil {
		return fmt.Errorf("set tag ref: %w", err)
	}
	return nil
}
