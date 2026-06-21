package core

import "sort"

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

func NewTree(entries []TreeEntry) *Tree {
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
	t.Hash = t.calculateHash()
	return t
}

func (t *Tree) calculateHash() string {
	data, err := t.Marshal()
	if err != nil {
		return ""
	}
	return CalculateHash(data)
}
