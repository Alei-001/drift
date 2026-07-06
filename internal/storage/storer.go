package storage

import "io"

// Storer is the composite interface for all storage backends.
type Storer interface {
	ChunkStorer
	SnapshotStorer
	ReferenceStorer
	IndexStorer
	PreviewStorer
	ConfigStorer
	io.Closer
}
