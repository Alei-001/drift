package storage

import (
	"context"

	"github.com/your-org/drift/internal/core"
)

// PreviewStorer provides access to preview (thumbnail) data.
type PreviewStorer interface {
	GetPreview(ctx context.Context, hash core.Hash, size int) ([]byte, error)
	PutPreview(ctx context.Context, hash core.Hash, size int, data []byte) error
}
