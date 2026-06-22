package core

import (
	"errors"
	"time"
)

var (
	ErrEntryNotFound = errors.New("entry not found")
)

type Index struct {
	Entries []IndexEntry
	byPath  map[string]int
}

type IndexEntry struct {
	Path       string
	Hash       string
	ModifiedAt time.Time
	Size       int64
	Mode       uint32
}

func (idx *Index) buildIndex() {
	idx.byPath = make(map[string]int, len(idx.Entries))
	for i, e := range idx.Entries {
		idx.byPath[e.Path] = i
	}
}

func (idx *Index) Entry(path string) (IndexEntry, error) {
	if idx.byPath == nil {
		idx.buildIndex()
	}
	i, ok := idx.byPath[path]
	if !ok {
		return IndexEntry{}, ErrEntryNotFound
	}
	return idx.Entries[i], nil
}

func (idx *Index) Has(path string) bool {
	if idx.byPath == nil {
		idx.buildIndex()
	}
	_, ok := idx.byPath[path]
	return ok
}

func (idx *Index) Add(entry IndexEntry) {
	if idx.byPath == nil {
		idx.buildIndex()
	}
	if i, ok := idx.byPath[entry.Path]; ok {
		idx.Entries[i] = entry
		return
	}
	idx.Entries = append(idx.Entries, entry)
	idx.byPath[entry.Path] = len(idx.Entries) - 1
}

func (idx *Index) Remove(path string) {
	if idx.byPath == nil {
		idx.buildIndex()
	}
	i, ok := idx.byPath[path]
	if !ok {
		return
	}
	idx.Entries = append(idx.Entries[:i], idx.Entries[i+1:]...)
	idx.byPath = nil
}

func (idx *Index) Clear() {
	idx.Entries = nil
	idx.byPath = nil
}
