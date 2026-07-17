package store

import (
	"io"
)

// StoreSet aggregates individual storage interfaces. Each field is an
// orthogonal concern that can be satisfied independently. Nil fields are
// allowed for backends that don't implement a particular concern (e.g. an
// in-memory backend may leave Compactor nil).
type StoreSet struct {
	Chunks    ChunkStorer
	Snapshots SnapshotStorer
	Refs      ReferenceStorer
	Index     IndexStorer
	Config    ConfigStorer
	Compactor ChunkCompactor

	closers []io.Closer
}

// NewStoreSet creates a StoreSet from a composite Storer. The sub-interfaces
// are extracted via type assertion; fields that the backend does not
// implement remain nil.
func NewStoreSet(s Storer) *StoreSet {
	ss := &StoreSet{
		Chunks:    s,
		Snapshots: s,
		Refs:      s,
		Index:     s,
		Config:    s,
	}
	if c, ok := s.(ChunkCompactor); ok {
		ss.Compactor = c
	}
	return ss
}

// AddCloser adds an io.Closer to the StoreSet's lifecycle. All registered
// closers are called when StoreSet.Close() is invoked. This is used by
// middleware (e.g. cache) to participate in cleanup without coupling the
// StoreSet to any specific implementation.
func (ss *StoreSet) AddCloser(c io.Closer) {
	ss.closers = append(ss.closers, c)
}

// Close calls Close() on every registered closer and on every field that
// implements io.Closer, then resets all fields to nil. Multiple calls are
// safe.
func (ss *StoreSet) Close() error {
	var errs []error
	for _, c := range ss.closers {
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	ss.closers = nil

	closers := []io.Closer{
		closable(ss.Chunks),
		closable(ss.Snapshots),
		closable(ss.Refs),
		closable(ss.Index),
		closable(ss.Config),
	}
	for _, c := range closers {
		if c != nil {
			if err := c.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	ss.Chunks = nil
	ss.Snapshots = nil
	ss.Refs = nil
	ss.Index = nil
	ss.Config = nil
	ss.Compactor = nil

	if len(errs) > 0 {
		return &closeError{errs: errs}
	}
	return nil
}

type closeError struct {
	errs []error
}

func (e *closeError) Error() string {
	return "store: close errors"
}

func (e *closeError) Unwrap() []error {
	return e.errs
}

func closable(v any) io.Closer {
	c, _ := v.(io.Closer)
	return c
}
