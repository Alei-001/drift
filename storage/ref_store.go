package storage

import "github.com/your-org/drift/core"

// ReferenceStorer provides access to reference storage.
type ReferenceStorer interface {
	GetRef(name string) (*core.Reference, error)
	SetRef(name string, ref *core.Reference) error
	ListRefs(prefix string) ([]*core.Reference, error)
	DeleteRef(name string) error
}
