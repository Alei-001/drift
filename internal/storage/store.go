package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

	config := map[string]interface{}{
		"version":        "1.0.0",
		"hash_algorithm": "sha256",
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	if err := os.WriteFile(filepath.Join(s.DriftDir(), configFile), data, 0644); err != nil {
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

	tmp := filepath.Join(s.DriftDir(), blobsDir, ".puttmp")
	dst, err := os.Create(tmp)
	if err != nil {
		return "", err
	}

	if _, err := dst.ReadFrom(src); err != nil {
		dst.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	dst.Close()

	data, err := os.ReadFile(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	hash := core.CalculateHash(data)
	path := s.blobPath(hash)

	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(tmp)
		return hash, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}

	return hash, nil
}

func (s *Store) PutTree(t *core.Tree) error {
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

	path := s.commitPath(c.ID)

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

		id := entry.Name()[:len(entry.Name())-4]
		c, err := s.GetCommit(id)
		if err != nil {
			return nil, fmt.Errorf("corrupted commit %s: %w", id, err)
		}
		commits = append(commits, c)
	}

	sort.Slice(commits, func(i, j int) bool {
		return commits[i].Timestamp.Before(commits[j].Timestamp)
	})

	return commits, nil
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
