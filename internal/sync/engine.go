package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
)

// localStore is the subset of storage.Store that the sync engine needs.
type localStore interface {
	HasObject(hash string) bool
	GetCommit(hash string) (*core.Commit, error)
	GetTree(hash string) (*core.Tree, error)
	GetRef(name string) (string, error)
	SaveRef(name string, hash string) error
	DriftDir() string
}

// Engine performs object-level push and pull against a remote Transport.
type Engine struct {
	transport Transport
	store     localStore
}

// NewEngine creates a sync engine for the given transport and local store.
func NewEngine(transport Transport, store localStore) *Engine {
	return &Engine{
		transport: transport,
		store:     store,
	}
}

// PushResult reports the outcome of a Push operation.
type PushResult struct {
	Branch string
	Pushed int
}

// FetchResult reports the outcome of a Fetch operation.
type FetchResult struct {
	Branch  string
	Fetched int
}

// PullResult reports the outcome of a Pull operation.
type PullResult struct {
	Branch string
	Pulled int
}

// trackingRef returns the local tracking ref name for a branch.
func trackingRef(branch string) string {
	return "remotes/origin/" + branch
}

// shortHash returns the first 8 chars of a hash, or the full string if shorter.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// Push uploads objects reachable from the local branch that the remote does
// not yet have, then updates the remote ref.
//
// Rejects if the remote ref has diverged from the local tracking ref (someone
// else pushed since the last sync). The caller must pull first.
func (e *Engine) Push(branch string) (*PushResult, error) {
	refName := "heads/" + branch
	localHash, err := e.store.GetRef(branch)
	if err != nil {
		return nil, fmt.Errorf("read local ref %s: %w", branch, err)
	}
	if localHash == "" {
		return nil, fmt.Errorf("branch %s has no commits", branch)
	}

	trackingHash, err := e.store.GetRef(trackingRef(refName))
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("read tracking ref: %w", err)
		}
	}

	remoteHash, err := e.transport.GetRef(refName)
	if err != nil {
		return nil, fmt.Errorf("read remote ref heads/%s: %w", branch, err)
	}
	if remoteHash != "" && remoteHash != trackingHash {
		return nil, fmt.Errorf(
			"remote branch %s has diverged (someone else pushed since your last sync)\n"+
				"  local tracking: %s\n  remote:         %s\n"+
				"  Run 'drift pull %s' first",
			branch, shortHash(trackingHash), shortHash(remoteHash), branch)
	}

	// Collect objects reachable from localHash but not from trackingHash.
	// All commits are local, so ReachableObjects can walk safely.
	objs, err := core.ReachableObjects(e.store, localHash, trackingHash)
	if err != nil {
		return nil, fmt.Errorf("collect reachable objects: %w", err)
	}

	var pushed int
	for hash, typ := range objs {
		key := objectPath(hash, typ.String())
		exists, err := e.transport.Exists(key)
		if err != nil {
			return nil, fmt.Errorf("check remote %s: %w", key, err)
		}
		if exists {
			continue
		}
		localPath := filepath.Join(e.store.DriftDir(), key)
		f, err := os.Open(localPath)
		if err != nil {
			return nil, fmt.Errorf("open local %s: %w", localPath, err)
		}
		if err := e.transport.Put(key, f); err != nil {
			f.Close()
			return nil, fmt.Errorf("put %s: %w", key, err)
		}
		f.Close()
		pushed++
	}

	if err := e.transport.PutRef(refName, localHash); err != nil {
		return nil, fmt.Errorf("update remote ref: %w", err)
	}

	if err := e.store.SaveRef(trackingRef(refName), localHash); err != nil {
		return nil, fmt.Errorf("save tracking ref: %w", err)
	}

	return &PushResult{Branch: branch, Pushed: pushed}, nil
}

// Fetch downloads new objects from the remote that are not yet in the local
// store, then updates the local tracking ref. It does not modify the working
// directory.
func (e *Engine) Fetch(branch string) (*FetchResult, error) {
	refName := "heads/" + branch
	remoteHash, err := e.transport.GetRef(refName)
	if err != nil {
		return nil, fmt.Errorf("read remote ref heads/%s: %w", branch, err)
	}
	if remoteHash == "" {
		return nil, fmt.Errorf("branch %s not found on remote", branch)
	}

	trackingHash, err := e.store.GetRef(trackingRef(refName))
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("read tracking ref: %w", err)
		}
	}
	if remoteHash == trackingHash {
		return &FetchResult{Branch: branch, Fetched: 0}, nil
	}

	// Download objects by iterating the commit chain from remoteHash back
	// to trackingHash, downloading each commit's tree and blobs.
	fetched, err := e.fetchChain(remoteHash, trackingHash)
	if err != nil {
		return nil, err
	}

	if err := e.store.SaveRef(trackingRef(refName), remoteHash); err != nil {
		return nil, fmt.Errorf("save tracking ref: %w", err)
	}

	return &FetchResult{Branch: branch, Fetched: fetched}, nil
}

// Pull is Fetch followed by updating the local branch ref. Does not update
// the working directory — the app layer handles that.
func (e *Engine) Pull(branch string) (*PullResult, error) {
	refName := "heads/" + branch
	result, err := e.Fetch(branch)
	if err != nil {
		return nil, err
	}
	if result.Fetched == 0 {
		return &PullResult{Branch: branch, Pulled: 0}, nil
	}

	remoteHash, err := e.transport.GetRef(refName)
	if err != nil {
		return nil, fmt.Errorf("verify remote ref after fetch: %w", err)
	}
	if err := e.store.SaveRef(branch, remoteHash); err != nil {
		return nil, fmt.Errorf("update local ref %s: %w", branch, err)
	}

	return &PullResult{Branch: branch, Pulled: result.Fetched}, nil
}

// Clone downloads all objects and refs from the remote.
func (e *Engine) Clone() error {
	refs, err := e.transport.ListRefs()
	if err != nil {
		return fmt.Errorf("list remote refs: %w", err)
	}
	if len(refs) == 0 {
		return fmt.Errorf("no refs found on remote (empty project?)")
	}

	for name, remoteHash := range refs {
		if err := e.fetchRef(name, remoteHash); err != nil {
			return err
		}
	}

	return nil
}

// fetchRef downloads all objects reachable from a single remote ref.
func (e *Engine) fetchRef(name, remoteHash string) error {
	if _, err := e.fetchChain(remoteHash, ""); err != nil {
		return fmt.Errorf("walk %s: %w", name, err)
	}
	if err := e.store.SaveRef(trackingRef(name), remoteHash); err != nil {
		return fmt.Errorf("save tracking ref %s: %w", name, err)
	}
	return nil
}

// fetchChain downloads objects by walking the commit chain from startHash
// backward to stopHash (exclusive). It downloads each commit, its tree, and
// all reachable blobs, then follows the parent chain.
func (e *Engine) fetchChain(startHash, stopHash string) (int, error) {
	var fetched int
	hash := startHash

	for hash != "" && hash != stopHash {
		// Download the commit if not present.
		if !e.store.HasObject(hash) {
			if err := e.saveRemoteObject(hash, "commit"); err != nil {
				return fetched, fmt.Errorf("download commit %s: %w", shortHash(hash), err)
			}
			fetched++
		}

		c, err := e.store.GetCommit(hash)
		if err != nil {
			return fetched, fmt.Errorf("get commit %s: %w", shortHash(hash), err)
		}

		// Download the tree if not present.
		if !e.store.HasObject(c.TreeHash) {
			if err := e.saveRemoteObject(c.TreeHash, "tree"); err != nil {
				return fetched, fmt.Errorf("download tree %s: %w", shortHash(c.TreeHash), err)
			}
			fetched++
		}

		// Walk the tree and download all blobs and subtrees.
		tree, err := e.store.GetTree(c.TreeHash)
		if err != nil {
			return fetched, fmt.Errorf("get tree %s: %w", shortHash(c.TreeHash), err)
		}
		n, err := e.fetchTree(tree)
		if err != nil {
			return fetched, err
		}
		fetched += n

		hash = c.Parent
	}

	return fetched, nil
}

// fetchTree recursively downloads all blobs and subtrees from a tree.
func (e *Engine) fetchTree(tree *core.Tree) (int, error) {
	var fetched int
	for _, entry := range tree.Entries {
		switch entry.Type {
		case core.BlobObject:
			if !e.store.HasObject(entry.Hash) {
				if err := e.saveRemoteObject(entry.Hash, "blob"); err != nil {
					return fetched, fmt.Errorf("download blob %s: %w", shortHash(entry.Hash), err)
				}
				fetched++
			}
		case core.TreeObject:
			if !e.store.HasObject(entry.Hash) {
				if err := e.saveRemoteObject(entry.Hash, "tree"); err != nil {
					return fetched, fmt.Errorf("download tree %s: %w", shortHash(entry.Hash), err)
				}
				fetched++
			}
			subTree, err := e.store.GetTree(entry.Hash)
			if err != nil {
				return fetched, fmt.Errorf("get subtree %s: %w", shortHash(entry.Hash), err)
			}
			n, err := e.fetchTree(subTree)
			if err != nil {
				return fetched, err
			}
			fetched += n
		}
	}
	return fetched, nil
}

// saveRemoteObject downloads an object from the remote and saves it locally.
func (e *Engine) saveRemoteObject(hash, typ string) error {
	key := objectPath(hash, typ)
	rc, err := e.transport.Get(key)
	if err != nil {
		return err
	}
	defer rc.Close()
	return e.saveObject(hash, key, rc)
}

// saveObject writes raw compressed bytes to the local .drift directory.
func (e *Engine) saveObject(hash, key string, data io.Reader) error {
	path := filepath.Join(e.store.DriftDir(), key)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
