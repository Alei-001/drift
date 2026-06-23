package core

import (
	"errors"
	"path"
)

// maxTreeDepth bounds recursive tree traversal to prevent stack overflow
// when reading a maliciously or accidentally deep-nested tree. Mirrors
// go-git's MaxTreeDepth default (4096, same as upstream Git's limit).
const maxTreeDepth = 4096

// ErrTreeTooDeep is returned when tree traversal exceeds maxTreeDepth.
var ErrTreeTooDeep = errors.New("tree nesting too deep")

// BlobEntry represents a single file discovered during tree traversal.
type BlobEntry struct {
	Path string
	Hash string
	Mode uint32
}

// DiffChange represents a single file change discovered during lazy tree diffing.
// Old is nil for added files; New is nil for deleted files.
type DiffChange struct {
	Path string
	Old  *BlobEntry // nil = added
	New  *BlobEntry // nil = deleted
}

type TreeReader struct {
	store StoreReader
}

type StoreReader interface {
	GetTree(hash string) (*Tree, error)
	GetBlob(hash string) ([]byte, error)
	GetBlobSize(hash string) (int64, error)
}

func NewTreeReader(store StoreReader) *TreeReader {
	return &TreeReader{store: store}
}

// ListBlobs recursively flattens the tree into a flat list of BlobEntry.
// Used by export, restore, switch — operations that need the full file list.
func (r *TreeReader) ListBlobs(tree *Tree, prefix string) ([]BlobEntry, error) {
	return r.listBlobs(tree, prefix, 0)
}

func (r *TreeReader) listBlobs(tree *Tree, prefix string, depth int) ([]BlobEntry, error) {
	if depth > maxTreeDepth {
		return nil, ErrTreeTooDeep
	}

	var result []BlobEntry

	for _, entry := range tree.Entries {
		entryPath := path.Join(prefix, entry.Name)

		if entry.Type == BlobObject {
			result = append(result, BlobEntry{
				Path: entryPath,
				Hash: entry.Hash,
				Mode: entry.Mode,
			})
		} else if entry.Type == TreeObject {
			subTree, err := r.store.GetTree(entry.Hash)
			if err != nil {
				return nil, err
			}
			subBlobs, err := r.listBlobs(subTree, entryPath, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, subBlobs...)
		}
	}

	return result, nil
}

// DiffTrees computes the difference between two trees using the full flattening
// approach. Prefer LazyDiffTrees for performance on trees with few changes.
func (r *TreeReader) DiffTrees(oldTree, newTree *Tree) (deleted, added, modified []BlobEntry, err error) {
	oldBlobs, err := r.ListBlobs(oldTree, "")
	if err != nil {
		return nil, nil, nil, err
	}

	newBlobs, err := r.ListBlobs(newTree, "")
	if err != nil {
		return nil, nil, nil, err
	}

	oldMap := make(map[string]BlobEntry)
	for _, b := range oldBlobs {
		oldMap[b.Path] = b
	}

	newMap := make(map[string]BlobEntry)
	for _, b := range newBlobs {
		newMap[b.Path] = b
	}

	for path, oldEntry := range oldMap {
		if newEntry, exists := newMap[path]; !exists {
			deleted = append(deleted, oldEntry)
		} else if oldEntry.Hash != newEntry.Hash {
			modified = append(modified, newEntry)
		}
	}

	for path, newEntry := range newMap {
		if _, exists := oldMap[path]; !exists {
			added = append(added, newEntry)
		}
	}

	return deleted, added, modified, nil
}

// LazyDiffTrees computes the difference between two trees using a Merkletrie-style
// double-iterator approach. When two subtrees have the same hash, the entire
// subtree is skipped without recursion. This is dramatically faster than
// DiffTrees when only a few files have changed.
//
// B1: mirrors go-git's merkletrie/difftree.go approach — skip entire subtrees
// when their hash is identical.
func (r *TreeReader) LazyDiffTrees(oldTree, newTree *Tree) ([]DiffChange, error) {
	var changes []DiffChange
	if err := r.lazyDiff("", oldTree, newTree, &changes, 0); err != nil {
		return nil, err
	}
	return changes, nil
}

func (r *TreeReader) lazyDiff(prefix string, oldTree, newTree *Tree, changes *[]DiffChange, depth int) error {
	if depth > maxTreeDepth {
		return ErrTreeTooDeep
	}

	oi, ni := 0, 0
	oldEntries := oldTree.Entries
	newEntries := newTree.Entries

	for oi < len(oldEntries) || ni < len(newEntries) {
		var oldName, newName string
		if oi < len(oldEntries) {
			oldName = treeEntrySortName(&oldEntries[oi])
		}
		if ni < len(newEntries) {
			newName = treeEntrySortName(&newEntries[ni])
		}

		switch {
		case ni >= len(newEntries) || (oi < len(oldEntries) && oldName < newName):
			// Only in old tree — deleted.
			if err := r.collectRecursive(prefix, &oldEntries[oi], true, changes, depth); err != nil {
				return err
			}
			oi++

		case oi >= len(oldEntries) || (ni < len(newEntries) && newName < oldName):
			// Only in new tree — added.
			if err := r.collectRecursive(prefix, &newEntries[ni], false, changes, depth); err != nil {
				return err
			}
			ni++

		default:
			// Same name — compare.
			entryPath := path.Join(prefix, oldEntries[oi].Name)

			if oldEntries[oi].Type == TreeObject && newEntries[ni].Type == TreeObject {
				if oldEntries[oi].Hash == newEntries[ni].Hash {
					// Same subtree hash — skip entirely. This is the core
					// Merkletrie optimization.
				} else {
					// Different subtree — recurse.
					subOld, err := r.store.GetTree(oldEntries[oi].Hash)
					if err != nil {
						return err
					}
					subNew, err := r.store.GetTree(newEntries[ni].Hash)
					if err != nil {
						return err
					}
					if err := r.lazyDiff(entryPath, subOld, subNew, changes, depth+1); err != nil {
						return err
					}
				}
			} else if oldEntries[oi].Type == BlobObject && newEntries[ni].Type == BlobObject {
				be := BlobEntry{Path: entryPath, Hash: newEntries[ni].Hash, Mode: newEntries[ni].Mode}
				if oldEntries[oi].Hash != newEntries[ni].Hash {
					obe := BlobEntry{Path: entryPath, Hash: oldEntries[oi].Hash, Mode: oldEntries[oi].Mode}
					*changes = append(*changes, DiffChange{
						Path: entryPath,
						Old:  &obe,
						New:  &be,
					})
				}
			} else {
				// Type changed (blob↔tree).
				if oldEntries[oi].Type == TreeObject {
					if err := r.collectRecursive(prefix, &oldEntries[oi], true, changes, depth); err != nil {
						return err
					}
				} else {
					*changes = append(*changes, DiffChange{
						Path: entryPath,
						Old:  &BlobEntry{Path: entryPath, Hash: oldEntries[oi].Hash, Mode: oldEntries[oi].Mode},
					})
				}
				if newEntries[ni].Type == TreeObject {
					if err := r.collectRecursive(prefix, &newEntries[ni], false, changes, depth); err != nil {
						return err
					}
				} else {
					*changes = append(*changes, DiffChange{
						Path: entryPath,
						New:  &BlobEntry{Path: entryPath, Hash: newEntries[ni].Hash, Mode: newEntries[ni].Mode},
					})
				}
			}
			oi++
			ni++
		}
	}
	return nil
}

// collectRecursive collects all blobs under a tree entry into changes.
// If isDelete is true, only Old is set; otherwise only New is set.
func (r *TreeReader) collectRecursive(prefix string, entry *TreeEntry, isDelete bool, changes *[]DiffChange, depth int) error {
	if depth > maxTreeDepth {
		return ErrTreeTooDeep
	}
	entryPath := path.Join(prefix, entry.Name)

	if entry.Type == BlobObject {
		be := BlobEntry{Path: entryPath, Hash: entry.Hash, Mode: entry.Mode}
		if isDelete {
			*changes = append(*changes, DiffChange{Path: entryPath, Old: &be})
		} else {
			*changes = append(*changes, DiffChange{Path: entryPath, New: &be})
		}
		return nil
	}

	// TreeObject — open and recurse.
	subTree, err := r.store.GetTree(entry.Hash)
	if err != nil {
		return err
	}
	for i := range subTree.Entries {
		if err := r.collectRecursive(entryPath, &subTree.Entries[i], isDelete, changes, depth+1); err != nil {
			return err
		}
	}
	return nil
}
