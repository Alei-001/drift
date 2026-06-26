package core

import (
	"path"
	"strings"
)

type TreeBuilder struct {
	trees    map[string]*Tree
	store    func(*Tree) error
	baseTree *Tree       // old tree from parent commit (for subtree reuse)
	reader   StoreReader // used to load subtrees from baseTree
}

func NewTreeBuilder(storeFn func(*Tree) error) *TreeBuilder {
	return &TreeBuilder{
		trees: make(map[string]*Tree),
		store: storeFn,
	}
}

// BuildFromIndex builds a new tree from the index entries.
// For faster builds when a parent commit exists, use BuildFromIndexWithBase.
func (b *TreeBuilder) BuildFromIndex(idx *Index) (*Tree, error) {
	b.trees[""] = &Tree{}

	for i := range idx.Entries {
		b.addEntry(&idx.Entries[i])
	}

	return b.buildTree("")
}

// BuildFromIndexWithBase builds a new tree from the index, reusing unchanged
// subtrees from the parent commit's tree. When a subtree's entries (names,
// types, hashes) are identical to the parent's, the old subtree hash is reused
// and the subtree is not re-marshaled or re-compressed.
func (b *TreeBuilder) BuildFromIndexWithBase(idx *Index, baseTree *Tree, reader StoreReader) (*Tree, error) {
	b.trees = make(map[string]*Tree)
	b.trees[""] = &Tree{}
	b.baseTree = baseTree
	b.reader = reader

	for i := range idx.Entries {
		b.addEntry(&idx.Entries[i])
	}

	result, err := b.buildTree("")
	b.baseTree = nil
	b.reader = nil
	return result, err
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

	// Try to reuse the old subtree when it has identical entries, avoiding
	// Marshal + CalculateHash + PutTree for unchanged directories.
	if b.baseTree != nil && b.reader != nil {
		if oldSub, err := b.findBaseSubtree(treePath); err == nil && oldSub != nil {
			if treeEntriesEqual(t.Entries, oldSub.Entries) {
				result := &Tree{Hash: oldSub.Hash, Entries: t.Entries}
				if b.store != nil {
					if err := b.store(result); err != nil {
						return nil, err
					}
				}
				return result, nil
			}
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

// findBaseSubtree walks baseTree to find the subtree at the given path.
// Returns nil if the path doesn't exist in the base tree.
func (b *TreeBuilder) findBaseSubtree(treePath string) (*Tree, error) {
	if treePath == "" {
		return b.baseTree, nil
	}
	cur := b.baseTree
	parts := strings.Split(treePath, "/")
	for _, part := range parts {
		found := false
		for i := range cur.Entries {
			if cur.Entries[i].Name == part && cur.Entries[i].Type == TreeObject {
				sub, err := b.reader.GetTree(cur.Entries[i].Hash)
				if err != nil {
					return nil, err
				}
				cur = sub
				found = true
				break
			}
		}
		if !found {
			return nil, nil
		}
	}
	return cur, nil
}

// treeEntriesEqual reports whether two entry slices are identical
// in name, type, hash, and mode (sorted). Uses the same sort order
// as treeEntrySortName.
func treeEntriesEqual(a, b []TreeEntry) bool {
	if len(a) != len(b) {
		return false
	}
	// Both are already sorted by treeEntrySortName (enforced by NewTree / Validate).
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Type != b[i].Type ||
			a[i].Hash != b[i].Hash ||
			a[i].Mode != b[i].Mode {
			return false
		}
	}
	return true
}
