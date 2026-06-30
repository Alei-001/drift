package storage

import (
	"context"

	"github.com/your-org/drift/core"
)

// ReferenceStorer provides access to reference storage.
type ReferenceStorer interface {
	GetRef(ctx context.Context, name string) (*core.Reference, error)
	SetRef(ctx context.Context, name string, ref *core.Reference) error
	ListRefs(ctx context.Context, prefix string) ([]*core.Reference, error)
	DeleteRef(ctx context.Context, name string) error
}
