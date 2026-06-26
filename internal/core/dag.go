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
		if err := collectTreeObjects(store, tree, c.TreeHash, result); err != nil {
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
// hash is the known SHA-256 hash of this tree (tree.Hash is not set after
// Unmarshal because the DREE format doesn't store self-hashes).
func collectTreeObjects(store CommitTreeStore, tree *Tree, hash string, result map[string]ObjectType) error {
	result[hash] = TreeObject

	for _, entry := range tree.Entries {
		switch entry.Type {
		case BlobObject:
			result[entry.Hash] = BlobObject
		case TreeObject:
			subTree, err := store.GetTree(entry.Hash)
			if err != nil {
				return err
			}
			if err := collectTreeObjects(store, subTree, entry.Hash, result); err != nil {
				return err
			}
		}
	}

	return nil
}

// CollectReachable walks the commit DAG from every starting hash in
// startHashes and returns the union of all reachable object hashes
// with their types. Missing commits (e.g. from a corrupted reflog entry)
// are silently skipped so GC can still proceed on good data.
func CollectReachable(store CommitTreeStore, startHashes []string) map[string]ObjectType {
	result := make(map[string]ObjectType)

	for _, hash := range startHashes {
		if hash == "" {
			continue
		}
		// Best-effort: skip commits that can't be found.
		if _, err := store.GetCommit(hash); err != nil {
			continue
		}
		objs, err := ReachableObjects(store, hash, "")
		if err != nil {
			continue
		}
		for k, v := range objs {
			result[k] = v
		}
	}

	return result
}
