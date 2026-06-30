package filesystem

import (
	"context"

	"github.com/your-org/drift/core"
)

// GetPreview is a noop stub (Phase 1).
func (fs *FSStorage) GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error) {
	return nil, nil
}

// PutPreview is a noop stub (Phase 1).
func (fs *FSStorage) PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error {
	return nil
}
