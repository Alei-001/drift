package core

import (
	"path"
	"strings"
)

type TreeBuilder struct {
	trees   map[string]*Tree
	entries map[string]TreeEntry
}

func NewTreeBuilder() *TreeBuilder {
	return &TreeBuilder{
		trees:   make(map[string]*Tree),
		entries: make(map[string]TreeEntry),
	}
}

func (b *TreeBuilder) BuildFromIndex(idx *Index) *Tree {
	b.trees[""] = &Tree{}
	b.entries = make(map[string]TreeEntry)

	for i := range idx.Entries {
		b.addEntry(&idx.Entries[i])
	}

	return b.buildTree("")
}

func (b *TreeBuilder) addEntry(entry *IndexEntry) {
	parts := strings.Split(entry.Path, "/")

	var fullpath string
	for _, part := range parts {
		parent := fullpath
		fullpath = path.Join(fullpath, part)

		if fullpath == entry.Path {
			b.entries[fullpath] = TreeEntry{
				Name: part,
				Type: BlobObject,
				Hash: entry.Hash,
				Mode: entry.Mode,
			}
			b.getOrCreateTree(parent).Entries = append(
				b.getOrCreateTree(parent).Entries,
				b.entries[fullpath],
			)
		} else {
			if _, exists := b.trees[fullpath]; !exists {
				b.trees[fullpath] = &Tree{}
				te := TreeEntry{
					Name: part,
					Type: TreeObject,
				}
				b.getOrCreateTree(parent).Entries = append(
					b.getOrCreateTree(parent).Entries,
					te,
				)
			}
		}
	}
}

func (b *TreeBuilder) getOrCreateTree(path string) *Tree {
	if t, ok := b.trees[path]; ok {
		return t
	}
	t := &Tree{}
	b.trees[path] = t
	return t
}

func (b *TreeBuilder) buildTree(treePath string) *Tree {
	t := b.trees[treePath]
	if t == nil {
		return &Tree{}
	}

	for i := range t.Entries {
		if t.Entries[i].Type == TreeObject {
			subPath := path.Join(treePath, t.Entries[i].Name)
			subTree := b.buildTree(subPath)
			t.Entries[i].Hash = subTree.Hash
		}
	}

	result := NewTree(t.Entries)
	return result
}
