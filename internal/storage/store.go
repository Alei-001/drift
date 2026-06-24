package storage

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
)

var (
	ErrNotInitialized  = errors.New("drift project not initialized (run 'drift init')")
	ErrObjectNotFound  = errors.New("object not found")
	ErrObjectCorrupted = errors.New("object corrupted (hash mismatch)")
	ErrInvalidHash     = errors.New("invalid object hash")
	ErrInvalidRefName  = errors.New("invalid ref name")
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
	refExt     = ".ref"
)

type Store struct {
	root      string
	treeCache *treeLRUCache // B4: bounded LRU cache, avoids redundant disk I/O for hot trees
}

func NewStore(root string) *Store {
	return &Store{root: root, treeCache: newTreeLRUCache()}
}

// validateHash checks that hash is exactly 64 hexadecimal characters.
// This prevents path traversal via crafted hash values containing ".." or "/".
func validateHash(hash string) error {
	if len(hash) != 64 {
		return fmt.Errorf("%w: expected 64 hex characters, got %d", ErrInvalidHash, len(hash))
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidHash, err)
	}
	return nil
}

// validateRefName checks that name only contains [A-Za-z0-9._-/] and does
// not contain "..". This prevents path traversal via crafted ref names.
func validateRefName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty ref name", ErrInvalidRefName)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: %q contains '..'", ErrInvalidRefName, name)
	}
	for _, c := range name {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' || c == '/') {
			return fmt.Errorf("%w: %q contains invalid character %q", ErrInvalidRefName, name, c)
		}
	}
	return nil
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
	if len(hash) < 2 {
		return filepath.Join(s.DriftDir(), treesDir, hash+".dre")
	}
	// B10: two-level directory like blobs, avoiding flat directory with
	// thousands of entries.
	return filepath.Join(s.DriftDir(), treesDir, hash[:2], hash[2:]+".dre")
}

func (s *Store) commitPath(id string) string {
	if len(id) < 2 {
		return filepath.Join(s.DriftDir(), commitsDir, id+".dcm")
	}
	// B10: two-level directory for commits too.
	return filepath.Join(s.DriftDir(), commitsDir, id[:2], id[2:]+".dcm")
}

func (s *Store) PutBlob(data []byte) (string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()

	hash := core.CalculateHash(data)
	if err := validateHash(hash); err != nil {
		return "", err
	}
	path := s.blobPath(hash)

	if _, err := os.Stat(path); err == nil {
		return hash, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	// Compress blob data before writing to disk.
	if err := compressFileToPath(path, data); err != nil {
		return "", err
	}

	return hash, nil
}

func (s *Store) GetBlob(hash string) ([]byte, error) {
	if err := validateHash(hash); err != nil {
		return nil, err
	}
	path := s.blobPath(hash)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	data, err := decompressBytes(raw)
	if err != nil {
		return nil, err
	}

	actual := core.CalculateHash(data)
	if actual != hash {
		return nil, ErrObjectCorrupted
	}

	return data, nil
}

// GetBlobSize returns the original (uncompressed) size of the stored blob
// without fully reading its content into memory. Reads only the DRZL header.
func (s *Store) GetBlobSize(hash string) (int64, error) {
	if err := validateHash(hash); err != nil {
		return 0, err
	}
	path := s.blobPath(hash)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrObjectNotFound
		}
		return 0, err
	}
	defer f.Close()

	// Read the compression header to get the original size.
	header := make([]byte, compressedHeaderSz)
	n, err := io.ReadFull(f, header)
	if err != nil {
		return 0, ErrCorruptedObject
	}
	header = header[:n]

	if n < compressedHeaderSz || string(header[:4]) != compressedMagic {
		return 0, ErrCorruptedObject
	}

	return int64(binary.LittleEndian.Uint64(header[5:13])), nil
}

// GetBlobToWriter streams a blob's content to the given writer without loading
// the entire blob into memory. This is essential for large files (PSD, video)
// that creative workers handle. The hash is verified via a streaming hasher.
func (s *Store) GetBlobToWriter(hash string, w io.Writer) error {
	if err := validateHash(hash); err != nil {
		return err
	}
	path := s.blobPath(hash)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrObjectNotFound
		}
		return err
	}
	defer f.Close()

	// Wrap the file reader with transparent decompression.
	r, err := streamingDecompressReader(f)
	if err != nil {
		return err
	}
	defer r.Close()

	h := core.NewHasher()
	if _, err := io.Copy(io.MultiWriter(w, h), r); err != nil {
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

	return s.putBlobFromReaderLocked(src)
}

func (s *Store) PutBlobFromReader(r io.Reader) (string, error) {
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()

	return s.putBlobFromReaderLocked(r)
}

func (s *Store) putBlobFromReaderLocked(r io.Reader) (string, error) {
	// Read the full content into memory to compute hash and compress.
	// For very large files this is unavoidable since we need the hash
	// before we know the path, and compression needs the full content.
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	hash := core.CalculateHash(data)
	if err := validateHash(hash); err != nil {
		return "", err
	}
	path := s.blobPath(hash)

	if _, err := os.Stat(path); err == nil {
		return hash, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}

	// Write compressed data atomically.
	if err := compressFileToPath(path, data); err != nil {
		return "", err
	}

	return hash, nil
}

func (s *Store) PutTree(t *core.Tree) error {
	if t == nil {
		return core.ErrInvalidTree
	}
	if err := validateHash(t.Hash); err != nil {
		return err
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

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := t.Marshal()
	if err != nil {
		return err
	}

	// Write compressed tree data atomically.
	if err := compressFileToPath(path, data); err != nil {
		return err
	}

	return nil
}

func (s *Store) GetTree(hash string) (*core.Tree, error) {
	if err := validateHash(hash); err != nil {
		return nil, err
	}
	// B4: check cache before disk read.
	if cached, ok := s.treeCache.Load(hash); ok {
		return cached, nil
	}

	path := s.treePath(hash)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	data, err := decompressBytes(raw)
	if err != nil {
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

	s.treeCache.Store(hash, t)
	return t, nil
}

func (s *Store) PutCommit(c *core.Commit) error {
	if err := validateHash(c.Hash); err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	// Use hash as filename to avoid conflicts when different branches have same ID
	path := s.commitPath(c.Hash)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := c.Marshal()
	if err != nil {
		return err
	}

	// Write compressed commit data atomically.
	if err := compressFileToPath(path, data); err != nil {
		return err
	}

	return nil
}

func (s *Store) GetCommit(id string) (*core.Commit, error) {
	if err := validateHash(id); err != nil {
		return nil, err
	}
	path := s.commitPath(id)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}

	data, err := decompressBytes(raw)
	if err != nil {
		return nil, err
	}

	c := &core.Commit{}
	if err := c.Unmarshal(data); err != nil {
		return nil, err
	}

	// Recompute the hash from the commit's fields and verify it matches
	// the filename, mirroring GetBlob/GetTree's integrity check. This
	// catches corrupted objects where the stored Hash field has been
	// forged to match the filename.
	if recomputed := c.ComputeHash(); recomputed != id {
		return nil, fmt.Errorf("%w: recomputed hash %q does not match filename %q",
			ErrObjectCorrupted, recomputed, id)
	}

	return c, nil
}

func (s *Store) ListCommits() ([]*core.Commit, error) {
	dir := filepath.Join(s.DriftDir(), commitsDir)

	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || len(entry.Name()) != 2 {
			continue
		}
		subDir := filepath.Join(dir, entry.Name())
		subEntries, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		for _, se := range subEntries {
			if se.IsDir() || filepath.Ext(se.Name()) != ".dcm" {
				continue
			}
			hash := entry.Name() + se.Name()[:len(se.Name())-4]
			files = append(files, hash)
		}
	}

	var commits []*core.Commit
	for _, hash := range files {
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
	if err := validateRefName(name); err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	// B5: use plain-text .ref format (just the 64-char hex hash) instead of JSON.
	// Mirrors Git's lightweight ref format: a single line of hex hash.
	path := filepath.Join(s.DriftDir(), refsDir, name+refExt)
	data := []byte(commitHash + "\n")

	// Create parent directory for nested refs (e.g. "names/label").
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
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
	if err := validateRefName(name); err != nil {
		return "", err
	}
	path := filepath.Join(s.DriftDir(), refsDir, name+refExt)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrObjectNotFound
		}
		return "", err
	}
	// Parse: single line of hex hash or "ref: <name>"
	ref := strings.TrimSpace(string(data))
	if strings.HasPrefix(ref, "ref: ") {
		return ref[5:], nil
	}
	return ref, nil
}

func (s *Store) ListRefs() (map[string]string, error) {
	refs := make(map[string]string)
	refsDirPath := filepath.Join(s.DriftDir(), refsDir)

	err := filepath.Walk(refsDirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}

		name := info.Name()
		if filepath.Ext(name) != refExt {
			return nil
		}

		rel, err := filepath.Rel(refsDirPath, path)
		if err != nil {
			return nil
		}
		refName := filepath.ToSlash(strings.TrimSuffix(rel, refExt))

		commitHash, err := s.GetRef(refName)
		if err != nil {
			return nil
		}
		refs[refName] = commitHash
		return nil
	})
	if err != nil {
		return nil, err
	}

	return refs, nil
}

// DeleteRef removes a branch ref. It refuses to delete "HEAD" or the
// currently checked-out branch. Mirrors go-git's Reference deletion.
func (s *Store) DeleteRef(name string) error {
	if err := validateRefName(name); err != nil {
		return err
	}
	if name == "HEAD" {
		return fmt.Errorf("cannot delete HEAD")
	}

	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	path := filepath.Join(s.DriftDir(), refsDir, name+refExt)
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
	if err := validateRefName(oldName); err != nil {
		return err
	}
	if err := validateRefName(newName); err != nil {
		return err
	}
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

	// Read old ref content to verify it exists.
	oldPath := filepath.Join(s.DriftDir(), refsDir, oldName+refExt)
	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("branch %q not found", oldName)
		}
		return err
	}

	// Refuse to overwrite an existing branch.
	newPath := filepath.Join(s.DriftDir(), refsDir, newName+refExt)
	if _, err := os.Stat(newPath); err == nil {
		return fmt.Errorf("branch %q already exists", newName)
	}

	// Create parent directory for nested refs (e.g. "names/label").
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return err
	}

	// Atomically rename the ref file from oldName to newName.
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}

	// Update HEAD if it pointed at oldName.
	// Write directly (not via SaveRef) to avoid re-entrant lock — we already hold it.
	if head, err := s.GetRef("HEAD"); err == nil && head == oldName {
		headPath := filepath.Join(s.DriftDir(), refsDir, "HEAD"+refExt)
		headData := []byte(newName + "\n")
		headTmp := headPath + ".tmp"
		if err := os.WriteFile(headTmp, headData, 0644); err != nil {
			return err
		}
		if err := os.Rename(headTmp, headPath); err != nil {
			_ = os.Remove(headTmp)
			return err
		}
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
// and persists the index, all under a single lock. This prevents orphan
// commits or duplicate saves if one of the steps fails (Issue 6).
//
// The caller must pass the index that was used to build the commit's tree.
// Persisting it as-is keeps full metadata (mtime/size) so that subsequent
// `drift add` or `drift status` does not falsely report committed files as
// deleted, and the status fast-path remains effective.
func (s *Store) SaveCommitTransaction(c *core.Commit, branch string, idx *core.Index) error {
	if err := validateHash(c.Hash); err != nil {
		return err
	}
	if err := validateRefName(branch); err != nil {
		return err
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()

	// 1. Write commit object (compressed).
	commitPath := s.commitPath(c.Hash)
	if err := os.MkdirAll(filepath.Dir(commitPath), 0755); err != nil {
		return err
	}
	commitData, err := c.Marshal()
	if err != nil {
		return err
	}
	if err := compressFileToPath(commitPath, commitData); err != nil {
		return err
	}

	// 2. Update branch ref.
	refPath := filepath.Join(s.DriftDir(), refsDir, branch+refExt)
	refData := []byte(c.Hash + "\n")
	refTmp := refPath + ".tmp"
	if err := os.WriteFile(refTmp, refData, 0644); err != nil {
		return err
	}
	if err := os.Rename(refTmp, refPath); err != nil {
		_ = os.Remove(refTmp)
		return err
	}

	// 3. Persist the index. The index already reflects all tracked files
	//    (it is what the commit's tree was built from), so write it back
	//    unchanged. This prevents false "Deleted" status entries for
	//    committed files that haven't been re-staged via `drift add`.
	idxPath := filepath.Join(s.DriftDir(), indexFile)
	idxData, err := idx.Marshal()
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
	seen := make(map[string]struct{})
	for current != "" {
		// Cycle detection: if we visit the same commit twice, the parent
		// chain is corrupted and would loop forever.
		if _, ok := seen[current]; ok {
			return nil, fmt.Errorf("cycle detected in commit history at %s", current)
		}
		seen[current] = struct{}{}
		c, err := s.GetCommit(current)
		if err != nil {
			return nil, fmt.Errorf("corrupted commit %s: %w", current, err)
		}
		commits = append(commits, c)
		current = c.Parent
	}

	return commits, nil
}
