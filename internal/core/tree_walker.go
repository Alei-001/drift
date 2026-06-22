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

type BlobEntry struct {
	Path string
	Hash string
	Mode uint32
}

type TreeReader struct {
	store StoreReader
}

type StoreReader interface {
	GetTree(hash string) (*Tree, error)
	GetBlob(hash string) ([]byte, error)
}

func NewTreeReader(store StoreReader) *TreeReader {
	return &TreeReader{store: store}
}

func (r *TreeReader) ListBlobs(tree *Tree, prefix string) ([]BlobEntry, error) {
	return r.listBlobs(tree, prefix, 0)
}

// listBlobs is the depth-tracked implementation of ListBlobs. Tracking depth
// prevents a crafted tree (e.g. a/a/a/.../a) from exhausting the call stack.
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
