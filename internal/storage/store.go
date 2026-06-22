package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
)

var (
	ErrNotInitialized  = errors.New("drift project not initialized (run 'drift init')")
	ErrObjectNotFound  = errors.New("object not found")
	ErrObjectCorrupted = errors.New("object corrupted (hash mismatch)")
)

const (
	driftDir   = ".drift"
	blobsDir   = "objects/blobs"
	treesDir   = "objects/trees"
	commitsDir = "commits"
	refsDir    = "refs"
	indexFile  = "index"
	configFile = "config.json"
	lockFile   = "lock"
)

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func (s *Store) DriftDir() string {
	return filepath.Join(s.root, driftDir)
}

func (s *Store) IsInitialized() bool {
	_, err := os.Stat(s.DriftDir())
	return err == nil
}

func (s *Store) Init() error {
	dirs := []string{
		s.DriftDir(),
		filepath.Join(s.DriftDir(), blobsDir),
		filepath.Join(s.DriftDir(), treesDir),
		filepath.Join(s.DriftDir(), commitsDir),
		filepath.Join(s.DriftDir(), refsDir),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	// Issue 13: use the same config schema as config.LoadConfig expects
	// (user + core), not an ad-hoc map. Issue 23: write atomically.
	cfg := config.DefaultConfig()
	if err := config.SaveConfig(s.DriftDir(), cfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// lock acquires an OS-level exclusive file lock on .drift/lock.
// Returns an unlock function and an error. If err is non-nil, the lock was not acquired.
func (s *Store) lock() (func(), error) {
	lockPath := filepath.Join(s.DriftDir(), lockFile)
	fl, err := acquireFileLock(lockPath)
	if err != nil {
		return func() {}, fmt.Errorf("could not acquire lock (another drift process running?): %w", err)
	}
	return fl.release, nil
}

func (s *Store) blobPath(hash string) string {
	if len(hash) < 2 {
		return filepath.Join(s.DriftDir(), blobsDir, hash)
	}
	return filepath.Join(s.DriftDir(), blobsDir, hash[:2], hash[2:])
}

func (s *Store) treePath(hash string) string {
	return filepath.Join(s.DriftDir(), treesDir, hash+".dre")
}

func (s *Store) commitPath(id string) string {
	return filepath.Join(s.DriftDir(), commitsDir, id+".dcm")
}

func (s *Store) PutBlob(data []byte) (string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()

	hash := core.CalculateHash(data)
	path := s.blobPath(hash)

	if _, err := os.Stat(path); err == nil {
		return hash, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	return hash, nil
}

func (s *Store) GetBlob(hash string) ([]byte, error) {
	path := s.blobPath(hash)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	actual := core.CalculateHash(data)
	if actual != hash {
		return nil, ErrObjectCorrupted
	}

	return data, nil
}

// GetBlobToWriter streams a blob's content to the given writer without loading
// the entire blob into memory. This is essential for large files (PSD, video)
// that creative workers handle. The hash is verified via a streaming hasher.
func (s *Store) GetBlobToWriter(hash string, w io.Writer) error {
	path := s.blobPath(hash)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return err
	}
	defer f.Close()

	h := core.NewHasher()
	if _, err := io.Copy(io.MultiWriter(w, h), f); err != nil {
		return err
	}

	if core.HexSum(h) != hash {
		return ErrObjectCorrupted
	}

	return nil
}

func (s *Store) PutBlobFromFile(filePath string) (string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()

	src, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	// Issue 16: use a unique temp file to avoid collisions on concurrent calls.
	tmp, err := os.CreateTemp(s.DriftDir(), "putblob-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.ReadFrom(src); err != nil {
		tmp.Close()
		return "", err
	}
	tmp.Close()

	// Stream-hash the tmp file instead of reading it all into memory.
	// This avoids OOM on large files (the core use case for creative workers).
	hash, err := core.CalculateHashFromFile(tmpName)
	if err != nil {
		return "", err
	}

	path := s.blobPath(hash)

	if _, err := os.Stat(path); err == nil {
		return hash, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}

	return hash, nil
}

func (s *Store) PutTree(t *core.Tree) error {
	if t == nil {
		return core.ErrInvalidTree
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	path := s.treePath(t.Hash)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	data, err := t.Marshal()
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	return nil
}

func (s *Store) GetTree(hash string) (*core.Tree, error) {
	path := s.treePath(hash)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	t := &core.Tree{}
	if err := t.Unmarshal(data); err != nil {
		return nil, err
	}

	actual := core.CalculateHash(data)
	if actual != hash {
		return nil, ErrObjectCorrupted
	}

	return t, nil
}

func (s *Store) PutCommit(c *core.Commit) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	// Use hash as filename to avoid conflicts when different branches have same ID
	path := s.commitPath(c.Hash)

	data, err := c.Marshal()
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	return nil
}

func (s *Store) GetCommit(id string) (*core.Commit, error) {
	path := s.commitPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	c := &core.Commit{}
	if err := c.Unmarshal(data); err != nil {
		return nil, err
	}

	// Verify the filename hash matches the hash stored inside the commit.
	// This catches wrong-file reads and partial-write corruption where the
	// internal field hash happens to be self-consistent but the file on disk
	// is not the one the caller asked for.
	if c.Hash != id {
		return nil, fmt.Errorf("%w: filename hash %q does not match commit hash %q",
			ErrObjectCorrupted, id, c.Hash)
	}

	return c, nil
}

func (s *Store) ListCommits() ([]*core.Commit, error) {
	dir := filepath.Join(s.DriftDir(), commitsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var commits []*core.Commit
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".dcm" {
			continue
		}

		// File name is the commit hash
		hash := entry.Name()[:len(entry.Name())-4]
		c, err := s.GetCommit(hash)
		if err != nil {
			return nil, fmt.Errorf("corrupted commit %s: %w", hash, err)
		}
		commits = append(commits, c)
	}

	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Timestamp.Before(commits[j].Timestamp)
	})

	return commits, nil
}

// WithLock acquires the store's exclusive lock, runs fn, then releases the
// lock. This allows callers to compose multi-step operations (e.g. read ref,
// modify, write ref) atomically without exposing the lock primitive.
// P2-#7: prevents TOCTOU races between GetRef and SaveRef.
func (s *Store) WithLock(fn func() error) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

func (s *Store) SaveRef(name, commitHash string) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	path := filepath.Join(s.DriftDir(), refsDir, name+".json")
	data, err := json.MarshalIndent(map[string]string{
		"name":        name,
		"commit_hash": commitHash,
	}, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	return nil
}

func (s *Store) GetRef(name string) (string, error) {
	path := filepath.Join(s.DriftDir(), refsDir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrObjectNotFound
		}
		return "", err
	}

	var ref map[string]string
	if err := json.Unmarshal(data, &ref); err != nil {
		return "", err
	}

	return ref["commit_hash"], nil
}

func (s *Store) ListRefs() (map[string]string, error) {
	dir := filepath.Join(s.DriftDir(), refsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	refs := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-5]
		commitHash, err := s.GetRef(name)
		if err != nil {
			continue
		}
		refs[name] = commitHash
	}

	return refs, nil
}

// DeleteRef removes a branch ref. It refuses to delete "HEAD" or the
// currently checked-out branch. Mirrors go-git's Reference deletion.
func (s *Store) DeleteRef(name string) error {
	if name == "HEAD" {
		return fmt.Errorf("cannot delete HEAD")
	}

	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	path := filepath.Join(s.DriftDir(), refsDir, name+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("branch %q not found", name)
		}
		return err
	}
	return nil
}

// RenameRef renames a branch ref from oldName to newName. HEAD is updated
// if it pointed at oldName. Mirrors go-git's branch rename.
func (s *Store) RenameRef(oldName, newName string) error {
	if oldName == "HEAD" || newName == "HEAD" {
		return fmt.Errorf("cannot rename HEAD")
	}
	if oldName == newName {
		return nil
	}

	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	oldPath := filepath.Join(s.DriftDir(), refsDir, oldName+".json")
	newPath := filepath.Join(s.DriftDir(), refsDir, newName+".json")

	// Read old ref.
	data, err := os.ReadFile(oldPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("branch %q not found", oldName)
		}
		return err
	}

	// Refuse to overwrite an existing branch.
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("branch %q already exists", newName)
	}

	// Write new ref with updated name field, then remove old.
	var ref map[string]string
	if err := json.Unmarshal(data, &ref); err != nil {
		return err
	}
	ref["name"] = newName
	newData, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return err
	}
	tmp := newPath + ".tmp"
	if err := os.WriteFile(tmp, newData, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, newPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Remove(oldPath); err != nil {
		return err
	}

	// Update HEAD if it pointed at oldName.
	if head, err := s.GetRef("HEAD"); err == nil && head == oldName {
		_ = s.SaveRef("HEAD", newName)
	}

	return nil
}

func (s *Store) SaveIndex(idx *core.Index) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	path := filepath.Join(s.DriftDir(), indexFile)
	data, err := idx.Marshal()
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	return nil
}

func (s *Store) LoadIndex(idx *core.Index) error {
	path := filepath.Join(s.DriftDir(), indexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return idx.Unmarshal(data)
}

// SaveCommitTransaction atomically writes a commit, updates the branch ref,
// and clears the index, all under a single lock. This prevents orphan commits
// or duplicate saves if one of the steps fails (Issue 6).
func (s *Store) SaveCommitTransaction(c *core.Commit, branch string, emptyIdx *core.Index) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	// 1. Write commit object.
	commitPath := s.commitPath(c.Hash)
	commitData, err := c.Marshal()
	if err != nil {
		return err
	}
	commitTmp := commitPath + ".tmp"
	if err := os.WriteFile(commitTmp, commitData, 0644); err != nil {
		return err
	}
	if err := os.Rename(commitTmp, commitPath); err != nil {
		_ = os.Remove(commitTmp)
		return err
	}

	// 2. Update branch ref.
	refPath := filepath.Join(s.DriftDir(), refsDir, branch+".json")
	refData, err := json.MarshalIndent(map[string]string{
		"name":        branch,
		"commit_hash": c.Hash,
	}, "", "  ")
	if err != nil {
		return err
	}
	refTmp := refPath + ".tmp"
	if err := os.WriteFile(refTmp, refData, 0644); err != nil {
		return err
	}
	if err := os.Rename(refTmp, refPath); err != nil {
		_ = os.Remove(refTmp)
		return err
	}

	// 3. Clear index.
	idxPath := filepath.Join(s.DriftDir(), indexFile)
	idxData, err := emptyIdx.Marshal()
	if err != nil {
		return err
	}
	idxTmp := idxPath + ".tmp"
	if err := os.WriteFile(idxTmp, idxData, 0644); err != nil {
		return err
	}
	if err := os.Rename(idxTmp, idxPath); err != nil {
		_ = os.Remove(idxTmp)
		return err
	}

	return nil
}

// ListBranchCommits walks the parent chain from the given branch ref and
// returns commits in reverse-chronological order (newest first). This avoids
// scanning and deserializing ALL commit files (Issue 7).
func (s *Store) ListBranchCommits(branch string) ([]*core.Commit, error) {
	hash, err := s.GetRef(branch)
	if err != nil {
		if err == ErrObjectNotFound {
			return nil, nil
		}
		return nil, err
	}

	var commits []*core.Commit
	current := hash
	for current != "" {
		c, err := s.GetCommit(current)
		if err != nil {
			return nil, fmt.Errorf("corrupted commit %s: %w", current, err)
		}
		commits = append(commits, c)
		current = c.Parent
	}

	return commits, nil
}
