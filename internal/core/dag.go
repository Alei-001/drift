package core

// CommitTreeStore is the minimal interface needed to walk the commit DAG.
// storage.Store satisfies this via its GetCommit and GetTree methods.
type CommitTreeStore interface {
	GetCommit(hash string) (*Commit, error)
	GetTree(hash string) (*Tree, error)
}

// ReachableObjects collects all object hashes reachable from startHash along
// the commit DAG, stopping before stopHash (exclusive). It returns a map from
// hash to object type, suitable for push/pull operations.
//
// If stopHash is empty, all objects reachable from startHash are collected.
// If stopHash equals startHash, returns an empty map.
func ReachableObjects(store CommitTreeStore, startHash, stopHash string) (map[string]ObjectType, error) {
	result := make(map[string]ObjectType)
	if startHash == stopHash {
		return result, nil
	}

	queue := []string{startHash}
	seen := make(map[string]bool)

	for len(queue) > 0 {
		hash := queue[0]
		queue = queue[1:]

		if hash == stopHash {
			continue
		}
		if seen[hash] {
			continue
		}
		seen[hash] = true

		c, err := store.GetCommit(hash)
		if err != nil {
			return nil, err
		}

		result[hash] = CommitObject

		// Walk the tree to collect all tree and blob hashes.
		tree, err := store.GetTree(c.TreeHash)
		if err != nil {
			return nil, err
		}
		if err := collectTreeObjects(store, tree, result); err != nil {
			return nil, err
		}

		// Follow parent if it exists and we haven't reached the stop hash.
		if c.Parent != "" && c.Parent != stopHash {
			queue = append(queue, c.Parent)
		}
	}

	return result, nil
}

// collectTreeObjects recursively collects all tree and blob hashes from a tree.
func collectTreeObjects(store CommitTreeStore, tree *Tree, result map[string]ObjectType) error {
	result[tree.Hash] = TreeObject

	for _, entry := range tree.Entries {
		switch entry.Type {
		case BlobObject:
			result[entry.Hash] = BlobObject
		case TreeObject:
			subTree, err := store.GetTree(entry.Hash)
			if err != nil {
				return err
			}
			if err := collectTreeObjects(store, subTree, result); err != nil {
				return err
			}
		}
	}

	return nil
}
