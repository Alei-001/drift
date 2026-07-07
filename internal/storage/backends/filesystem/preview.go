package filesystem

import (
	"context"
	"fmt"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// GetPreview is a noop stub (Phase 1). Returns ErrNotFound so callers can
// distinguish "no preview" from "preview feature disabled"; matches the
// memory backend's behavior.
func (fs *FSStorage) GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error) {
	return nil, fmt.Errorf("get preview %s: %w", hash.FullString(), storage.ErrNotFound)
}

// PutPreview is a noop stub (Phase 1).
func (fs *FSStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
	return nil
}
