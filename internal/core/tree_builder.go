package core

import (
	"path"
	"strings"
)

type TreeBuilder struct {
	trees map[string]*Tree
	store func(*Tree) error
}

func NewTreeBuilder(storeFn func(*Tree) error) *TreeBuilder {
	return &TreeBuilder{
		trees: make(map[string]*Tree),
		store: storeFn,
	}
}

func (b *TreeBuilder) BuildFromIndex(idx *Index) (*Tree, error) {
	b.trees[""] = &Tree{}

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
			te := TreeEntry{
				Name: part,
				Type: BlobObject,
				Hash: entry.Hash,
				Mode: entry.Mode,
			}
			b.getOrCreateTree(parent).Entries = append(
				b.getOrCreateTree(parent).Entries,
				te,
			)
		} else {
			if _, exists := b.trees[fullpath]; !exists {
				b.trees[fullpath] = &Tree{}
				te := TreeEntry{
					Name: part,
					Type: TreeObject,
					Mode: ModeDir,
				}
				b.getOrCreateTree(parent).Entries = append(
					b.getOrCreateTree(parent).Entries,
					te,
				)
			}
		}
	}
}

func (b *TreeBuilder) getOrCreateTree(treePath string) *Tree {
	if t, ok := b.trees[treePath]; ok {
		return t
	}
	t := &Tree{}
	b.trees[treePath] = t
	return t
}

func (b *TreeBuilder) buildTree(treePath string) (*Tree, error) {
	t := b.trees[treePath]
	if t == nil {
		return &Tree{}, nil
	}

	for i := range t.Entries {
		if t.Entries[i].Type == TreeObject {
			subPath := path.Join(treePath, t.Entries[i].Name)
			subTree, err := b.buildTree(subPath)
			if err != nil {
				return nil, err
			}
			t.Entries[i].Hash = subTree.Hash
		}
	}

	result, err := NewTree(t.Entries)
	if err != nil {
		return nil, err
	}

	if b.store != nil {
		if err := b.store(result); err != nil {
			return nil, err
		}
	}

	return result, nil
}
