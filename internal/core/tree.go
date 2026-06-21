package core

import (
	"encoding/json"
	"sort"
)

type TreeEntry struct {
	Name string     `json:"name"`
	Type ObjectType `json:"type"`
	Hash string     `json:"hash"`
}

type Tree struct {
	Hash    string      `json:"hash"`
	Entries []TreeEntry `json:"entries"`
}

// NewTree creates a new Tree with sorted entries. Returns nil if JSON marshaling fails.
func NewTree(entries []TreeEntry) *Tree {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	data, err := json.Marshal(entries)
	if err != nil {
		return nil
	}

	return &Tree{
		Hash:    CalculateHash(data),
		Entries: entries,
	}
}
