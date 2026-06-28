package storage

import "github.com/your-org/drift/core"

// IndexStorer provides access to the staging index.
type IndexStorer interface {
	GetIndex() (*core.Index, error)
	SetIndex(index *core.Index) error
}
