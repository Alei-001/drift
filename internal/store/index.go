package store

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
)

// IndexStorer provides access to the staging index.
type IndexStorer interface {
	GetIndex(ctx context.Context) (*core.Index, error)
	SetIndex(ctx context.Context, index *core.Index) error
}
