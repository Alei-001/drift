package core

import (
	"fmt"
	"sort"
)

type TreeEntry struct {
	Name string
	Type ObjectType
	Hash string
	Mode uint32
}

type Tree struct {
	Hash    string
	Entries []TreeEntry
}

func NewTree(entries []TreeEntry) (*Tree, error) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type == TreeObject && entries[j].Type != TreeObject {
			return true
		}
		if entries[i].Type != TreeObject && entries[j].Type == TreeObject {
			return false
		}
		return entries[i].Name < entries[j].Name
	})

	t := &Tree{Entries: entries}
	data, err := t.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tree: %w", err)
	}
	t.Hash = CalculateHash(data)
	return t, nil
}
