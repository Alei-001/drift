package core

import (
	"errors"
	"time"
)

var (
	ErrEntryNotFound = errors.New("entry not found")
)

type Index struct {
	Entries []IndexEntry `json:"entries"`
}

type IndexEntry struct {
	Path       string    `json:"path"`
	Hash       string    `json:"hash"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"`
}

func (idx *Index) Entry(path string) (IndexEntry, error) {
	for _, e := range idx.Entries {
		if e.Path == path {
			return e, nil
		}
	}
	return IndexEntry{}, ErrEntryNotFound
}

func (idx *Index) Has(path string) bool {
	_, err := idx.Entry(path)
	return err == nil
}

func (idx *Index) Add(entry IndexEntry) {
	for i, e := range idx.Entries {
		if e.Path == entry.Path {
			idx.Entries[i] = entry
			return
		}
	}
	idx.Entries = append(idx.Entries, entry)
}

func (idx *Index) Remove(path string) {
	for i, e := range idx.Entries {
		if e.Path == path {
			idx.Entries = append(idx.Entries[:i], idx.Entries[i+1:]...)
			return
		}
	}
}

func (idx *Index) Clear() {
	idx.Entries = nil
}
