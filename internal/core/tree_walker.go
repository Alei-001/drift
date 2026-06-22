package core

import "path"

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
			subBlobs, err := r.ListBlobs(subTree, entryPath)
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
