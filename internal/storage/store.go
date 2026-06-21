package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/drift/drift/internal/core"
)

var (
	ErrNotInitialized = errors.New("drift project not initialized (run 'drift init')")
	ErrObjectNotFound = errors.New("object not found")
)

const (
	driftDir    = ".drift"
	blobsDir    = "objects/blobs"
	treesDir    = "objects/trees"
	commitsDir  = "commits"
	refsDir     = "refs"
	indexFile   = "index"
	configFile  = "config.json"
	lockFile    = "lock"
)

type Store struct {
	root string
	mu   sync.Mutex
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

// lock acquires an in-process mutex and creates a lock file as a visual indicator.
// Note: This does NOT provide cross-process locking. For MVP this is sufficient.
func (s *Store) lock() func() {
	s.mu.Lock()
	lockPath := filepath.Join(s.DriftDir(), lockFile)
	_ = os.WriteFile(lockPath, []byte("locked"), 0644)
	return func() {
		_ = os.Remove(lockPath)
		s.mu.Unlock()
	}
}

func (s *Store) blobPath(hash string) string {
	if len(hash) < 2 {
		return filepath.Join(s.DriftDir(), blobsDir, hash)
	}
	return filepath.Join(s.DriftDir(), blobsDir, hash[:2], hash[2:])
}

func (s *Store) treePath(hash string) string {
	return filepath.Join(s.DriftDir(), treesDir, hash+".json")
}

func (s *Store) commitPath(id string) string {
	return filepath.Join(s.DriftDir(), commitsDir, id+".json")
}

func (s *Store) PutBlob(data []byte) (string, error) {
	unlock := s.lock()
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

	return hash, os.Rename(tmp, path)
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
	return data, nil
}

func (s *Store) PutBlobFromFile(filePath string) (string, error) {
	unlock := s.lock()
	defer unlock()

	hash, err := core.CalculateHashFromFile(filePath)
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

	src, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	tmp := path + ".tmp"
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

	return hash, os.Rename(tmp, path)
}

func (s *Store) PutTree(t *core.Tree) error {
	unlock := s.lock()
	defer unlock()

	path := s.treePath(t.Hash)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
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

	var t core.Tree
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}

	return &t, nil
}

func (s *Store) PutCommit(c *core.Commit) error {
	unlock := s.lock()
	defer unlock()

	path := s.commitPath(c.ID)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
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

	var c core.Commit
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}

	return &c, nil
}

func (s *Store) ListCommits() ([]*core.Commit, error) {
	dir := filepath.Join(s.DriftDir(), commitsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var commits []*core.Commit
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		id := entry.Name()[:len(entry.Name())-5]
		c, err := s.GetCommit(id)
		if err != nil {
			continue
		}
		commits = append(commits, c)
	}

	return commits, nil
}

func (s *Store) SaveRef(name, commitHash string) error {
	unlock := s.lock()
	defer unlock()

	path := filepath.Join(s.DriftDir(), refsDir, name+".json")
	data, err := json.MarshalIndent(map[string]string{
		"name":         name,
		"commit_hash": commitHash,
	}, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
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

func (s *Store) SaveIndex(idx *core.Index) error {
	unlock := s.lock()
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

	return os.Rename(tmp, path)
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
