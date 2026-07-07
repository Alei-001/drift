package memory

import (
	"context"
	"fmt"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

// GetPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error) {
	return nil, fmt.Errorf("get preview %s: %w", hash.FullString(), storage.ErrNotFound)
}

// PutPreview is a noop stub (Phase 1).
func (ms *MemoryStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
	return nil
}
