package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	dir       string // local project directory
}

// NewEngine creates a sync engine for the given transport and local store.
func NewEngine(transport Transport, store localStore, dir string) *Engine {
	return &Engine{
		transport: transport,
		store:     store,
		dir:       dir,
	}
}

// PushResult reports the outcome of a Push operation.
type PushResult struct {
	Branch string
	Pushed int // number of new objects uploaded
}

// FetchResult reports the outcome of a Fetch operation.
type FetchResult struct {
	Branch  string
	Fetched int // number of new objects downloaded
}

// PullResult reports the outcome of a Pull operation.
type PullResult struct {
	Branch string
	Pulled int // number of new objects downloaded
}

// trackingRef returns the local tracking ref name for a branch.
func trackingRef(branch string) string {
	return "remotes/origin/" + branch
}

// Push uploads objects reachable from the local branch that the remote does
// not yet have, then updates the remote ref.
//
// Rejects if the remote ref has diverged from the local tracking ref (someone
// else pushed since the last sync). The caller must pull first.
func (e *Engine) Push(branch string) (*PushResult, error) {
	localHash, err := e.store.GetRef(branch)
	if err != nil {
		return nil, fmt.Errorf("read local ref %s: %w", branch, err)
	}
	if localHash == "" {
		return nil, fmt.Errorf("branch %s has no commits", branch)
	}

	trackingHash, _ := e.store.GetRef(trackingRef(branch))

	// Read the remote ref.
	remoteHash, err := e.transport.GetRef("heads/" + branch)
	if err != nil {
		return nil, fmt.Errorf("read remote ref heads/%s: %w", branch, err)
	}
	if remoteHash != "" && remoteHash != trackingHash {
		return nil, fmt.Errorf(
			"remote branch %s has diverged (someone else pushed since your last sync)\n"+
				"  local tracking: %s\n  remote:         %s\n"+
				"  Run 'drift pull %s' first",
			branch, trackingHash[:8], remoteHash[:8], branch)
	}

	// Collect objects reachable from localHash but not from trackingHash.
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

		// Read the raw compressed object from the local store.
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

	// Update remote ref.
	if err := e.transport.PutRef("heads/"+branch, localHash); err != nil {
		return nil, fmt.Errorf("update remote ref: %w", err)
	}

	// Update local tracking ref.
	if err := e.store.SaveRef(trackingRef(branch), localHash); err != nil {
		return nil, fmt.Errorf("save tracking ref: %w", err)
	}

	return &PushResult{Branch: branch, Pushed: pushed}, nil
}

// Fetch downloads new objects from the remote that are not yet in the local
// store, then updates the local tracking ref. It does not modify the working
// directory.
func (e *Engine) Fetch(branch string) (*FetchResult, error) {
	// Read remote ref.
	remoteHash, err := e.transport.GetRef("heads/" + branch)
	if err != nil {
		return nil, fmt.Errorf("read remote ref heads/%s: %w", branch, err)
	}
	if remoteHash == "" {
		return nil, fmt.Errorf("branch %s not found on remote", branch)
	}

	trackingHash, _ := e.store.GetRef(trackingRef(branch))
	if remoteHash == trackingHash {
		return &FetchResult{Branch: branch, Fetched: 0}, nil
	}

	// Collect objects reachable from remoteHash but not from trackingHash.
	objs, err := core.ReachableObjects(e.store, remoteHash, trackingHash)
	if err != nil {
		return nil, fmt.Errorf("collect reachable objects: %w", err)
	}

	var fetched int
	for hash, typ := range objs {
		if e.store.HasObject(hash) {
			continue
		}

		key := objectPath(hash, typ.String())
		rc, err := e.transport.Get(key)
		if err != nil {
			return nil, fmt.Errorf("get %s: %w", key, err)
		}

		if err := e.saveObject(hash, key, rc); err != nil {
			rc.Close()
			return nil, err
		}
		rc.Close()
		fetched++
	}

	// Update tracking ref.
	if err := e.store.SaveRef(trackingRef(branch), remoteHash); err != nil {
		return nil, fmt.Errorf("save tracking ref: %w", err)
	}

	return &FetchResult{Branch: branch, Fetched: fetched}, nil
}

// Pull is Fetch followed by updating the local branch ref. Does not update
// the working directory — the app layer handles that.
func (e *Engine) Pull(branch string) (*PullResult, error) {
	result, err := e.Fetch(branch)
	if err != nil {
		return nil, err
	}
	if result.Fetched == 0 {
		return &PullResult{Branch: branch, Pulled: 0}, nil
	}

	// Fast-forward the local branch.
	remoteHash, _ := e.transport.GetRef("heads/" + branch)
	if err := e.store.SaveRef(branch, remoteHash); err != nil {
		return nil, fmt.Errorf("update local ref %s: %w", branch, err)
	}

	return &PullResult{Branch: branch, Pulled: result.Fetched}, nil
}

// Clone downloads all objects and refs from the remote, equivalent to a
// full Fetch for every branch plus checkout. The working directory is NOT
// populated by this method — that is handled by the app layer.
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
	objs, err := core.ReachableObjects(e.store, remoteHash, "")
	if err != nil {
		return fmt.Errorf("walk %s: %w", name, err)
	}

	for hash, typ := range objs {
		if e.store.HasObject(hash) {
			continue
		}

		key := objectPath(hash, typ.String())
		rc, err := e.transport.Get(key)
		if err != nil {
			return fmt.Errorf("get %s: %w", key, err)
		}

		if err := e.saveObject(hash, key, rc); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}

	// Write the tracking ref locally.
	if err := e.store.SaveRef(trackingRef(name), remoteHash); err != nil {
		return fmt.Errorf("save tracking ref %s: %w", name, err)
	}

	return nil
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
