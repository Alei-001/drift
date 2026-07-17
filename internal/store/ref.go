package store

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
)

// ReferenceStorer provides access to reference store.
type ReferenceStorer interface {
	GetRef(ctx context.Context, name string) (*core.Reference, error)
	SetRef(ctx context.Context, name string, ref *core.Reference) error
	ListRefs(ctx context.Context, prefix string) ([]*core.Reference, error)
	DeleteRef(ctx context.Context, name string) error
}
