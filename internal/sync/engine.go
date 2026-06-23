// Package sync provides remote synchronization for drift projects.
//
// The sync engine supports multiple transports (local filesystem, WebDAV)
// behind a common Transport interface. Synchronization is incremental
// (content-hash based) and tracks deletions via a manifest file stored on
// the remote.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Transport is the interface for remote storage backends.
// All paths are forward-slash relative paths from the project root.
type Transport interface {
	// Get retrieves a file from the remote and writes it to dst.
	// Returns an error wrapping os.ErrNotExist if the file doesn't exist.
	Get(remotePath string, dst io.Writer) error

	// Put uploads src to the remote at remotePath.
	Put(remotePath string, src io.Reader) error

	// Stat returns metadata about a remote file, or os.ErrNotExist.
	Stat(remotePath string) (*RemoteStat, error)

	// List returns all files under the given remote directory prefix.
	// Paths are forward-slash relative, without a leading slash.
	List(prefix string) ([]string, error)

	// Delete removes a file from the remote. Removing a non-existent
	// file is not an error.
	Delete(remotePath string) error

	// Mkdir creates a directory on the remote (recursively).
	Mkdir(remotePath string) error
}

// RemoteStat holds metadata about a remote file.
type RemoteStat struct {
	Size    int64
	ModTime time.Time
}

// Manifest is the sync state file stored on the remote at
// .drift/sync/manifest.json. It records the last-known state of both
// the local and remote file trees, enabling incremental sync and
// deletion tracking.
type Manifest struct {
	ProjectID string             `json:"project_id"`
	Files     map[string]string  `json:"files"`      // path → content hash
	UpdatedAt string             `json:"updated_at"`
}

// newManifest creates an empty manifest.
func newManifest(projectID string) *Manifest {
	return &Manifest{
		ProjectID: projectID,
		Files:     make(map[string]string),
	}
}

// manifestPath is the remote path where the manifest is stored.
const manifestPath = ".drift/sync/manifest.json"

// fileHash computes the SHA-256 hash of a local file.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// scanFiles walks the local project directory and returns a map of
// forward-slash relative paths to content hashes. The .drift/lock and
// .drift/sync/ directories are excluded.
func scanFiles(rootDir string) (map[string]string, error) {
	files := make(map[string]string)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}

		// Skip the lock file and sync state directory.
		if rel == ".drift/lock" {
			return nil
		}
		if strings.HasPrefix(rel, ".drift/sync/") {
			return nil
		}

		// Skip symlinks for safety.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		hash, err := fileHash(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", rel, err)
		}
		files[rel] = hash
		return nil
	})

	return files, err
}

// SyncResult summarizes what happened during a sync operation.
type SyncResult struct {
	Pushed   []string // paths uploaded to remote
	Pulled   []string // paths downloaded from remote
	Deleted  []string // paths deleted (on the side that lost the file)
	RemoteDeleted []string // paths deleted on remote during push
	LocalDeleted  []string // paths deleted on local during pull
}

// HasChanges reports whether the sync did anything.
func (r *SyncResult) HasChanges() bool {
	return len(r.Pushed) > 0 || len(r.Pulled) > 0 ||
		len(r.RemoteDeleted) > 0 || len(r.LocalDeleted) > 0
}

// Engine performs bidirectional incremental sync between a local project
// directory and a remote Transport.
type Engine struct {
	transport Transport
	projectID string
}

// NewEngine creates a sync engine for the given transport and project ID.
func NewEngine(transport Transport, projectID string) *Engine {
	return &Engine{
		transport: transport,
		projectID: projectID,
	}
}

// Sync performs a bidirectional sync. The algorithm:
//  1. Load the last-known manifest from the remote (if any).
//  2. Scan the local file tree.
//  3. List the remote file tree.
//  4. Push: upload local files that are new or changed vs. manifest.
//  5. Push deletions: delete remote files that were in manifest but not local.
//  6. Pull: download remote files that are new or changed vs. manifest.
//  7. Pull deletions: delete local files that were in manifest but not remote.
//  8. Save the updated manifest to the remote.
//
// Conflict policy: if a file changed on both sides since the manifest, the
// local version wins (push overwrites remote). The remote's version is lost
// unless the user saved it as a commit. This is the "last save wins" model
// documented in the design.
func (e *Engine) Sync(localDir string) (*SyncResult, error) {
	result := &SyncResult{}

	// 1. Load manifest from remote.
	manifest := newManifest(e.projectID)
	if data, err := e.getRemoteFile(manifestPath); err == nil {
		if err := json.Unmarshal(data, manifest); err != nil {
			return nil, fmt.Errorf("invalid remote manifest: %w", err)
		}
	}
	// If manifest doesn't exist, that's fine — first sync.

	// 2. Scan local files.
	localFiles, err := scanFiles(localDir)
	if err != nil {
		return nil, fmt.Errorf("scan local: %w", err)
	}

	// 3. List remote files (excluding the manifest itself).
	remoteFiles, err := e.transport.List("")
	if err != nil {
		return nil, fmt.Errorf("list remote: %w", err)
	}
	remoteSet := make(map[string]bool, len(remoteFiles))
	for _, p := range remoteFiles {
		if p == manifestPath {
			continue
		}
		remoteSet[p] = true
	}

	// 4. Push: upload new/changed local files.
	// Also detect files that were deleted on the remote (in manifest, local
	// has them, but remote doesn't) — these should be deleted locally, not
	// re-pushed.
	pushedSet := make(map[string]bool)
	for path, localHash := range localFiles {
		manifestHash, inManifest := manifest.Files[path]
		remoteHas := remoteSet[path]

		// If the file was in the manifest (previously synced) but is no
		// longer on the remote, it was deleted on the remote side. Delete
		// it locally instead of re-pushing.
		if inManifest && !remoteHas {
			fullPath := filepath.Join(localDir, filepath.FromSlash(path))
			if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("delete local %s (remote-deleted): %w", path, err)
			}
			result.LocalDeleted = append(result.LocalDeleted, path)
			continue
		}

		if remoteHas && inManifest && manifestHash == localHash {
			// Unchanged — skip.
			continue
		}

		// Need to upload.
		fullPath := filepath.Join(localDir, filepath.FromSlash(path))
		if err := e.putLocalFile(path, fullPath); err != nil {
			return nil, fmt.Errorf("push %s: %w", path, err)
		}
		result.Pushed = append(result.Pushed, path)
		pushedSet[path] = true
	}

	// 5. Push deletions: files in manifest but not in local.
	for path := range manifest.Files {
		if _, exists := localFiles[path]; exists {
			continue
		}
		if remoteSet[path] {
			if err := e.transport.Delete(path); err != nil {
				return nil, fmt.Errorf("delete remote %s: %w", path, err)
			}
			result.RemoteDeleted = append(result.RemoteDeleted, path)
		}
	}

	// 6. Pull: download new/changed remote files.
	// Rebuild remoteSet since we may have deleted some files.
	remoteFiles, err = e.transport.List("")
	if err != nil {
		return nil, fmt.Errorf("list remote (after push): %w", err)
	}
	remoteSet = make(map[string]bool, len(remoteFiles))
	for _, p := range remoteFiles {
		if p == manifestPath {
			continue
		}
		remoteSet[p] = true
	}

	for path := range remoteSet {
		// Skip files we just pushed — local already has the latest version.
		if pushedSet[path] {
			continue
		}

		localHash, localHas := localFiles[path]
		manifestHash, inManifest := manifest.Files[path]

		if localHas && inManifest && manifestHash == localHash {
			// Unchanged — skip.
			continue
		}

		// Need to download.
		// If local doesn't have it, always pull.
		// If local has it but manifest says different, pull remote version.
		fullPath := filepath.Join(localDir, filepath.FromSlash(path))
		if err := e.getRemoteToFile(path, fullPath); err != nil {
			return nil, fmt.Errorf("pull %s: %w", path, err)
		}
		result.Pulled = append(result.Pulled, path)
		_ = localHash // unused but kept for clarity
	}

	// 7. Pull deletions are already handled in step 4 (when a file is in
	// the manifest but not on the remote, it's deleted locally during the
	// push scan). Nothing more to do here.

	// 8. Build the new manifest from the merged state.
	// After sync, local and remote should match. Use local as source of truth.
	newManifest := newManifest(e.projectID)
	for path, hash := range localFiles {
		newManifest.Files[path] = hash
	}
	// Add pulled files.
	for _, path := range result.Pulled {
		fullPath := filepath.Join(localDir, filepath.FromSlash(path))
		if hash, err := fileHash(fullPath); err == nil {
			newManifest.Files[path] = hash
		}
	}
	// Remove deleted files.
	for _, path := range result.LocalDeleted {
		delete(newManifest.Files, path)
	}
	for _, path := range result.RemoteDeleted {
		// Remote deleted means local also doesn't have it (step 5).
		delete(newManifest.Files, path)
	}

	newManifest.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := e.saveManifest(newManifest); err != nil {
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	// Sort results for deterministic output.
	sort.Strings(result.Pushed)
	sort.Strings(result.Pulled)
	sort.Strings(result.RemoteDeleted)
	sort.Strings(result.LocalDeleted)

	return result, nil
}

// getRemoteFile downloads a remote file into memory.
func (e *Engine) getRemoteFile(remotePath string) ([]byte, error) {
	var buf strings.Builder
	if err := e.transport.Get(remotePath, &buf); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// getRemoteToFile downloads a remote file to a local path.
func (e *Engine) getRemoteToFile(remotePath, localPath string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	tmp := localPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := e.transport.Get(remotePath, f); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	// Close before rename to release the file handle on Windows.
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, localPath)
}

// putLocalFile uploads a local file to the remote.
func (e *Engine) putLocalFile(remotePath, localPath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return e.transport.Put(remotePath, f)
}

// saveManifest uploads the manifest to the remote.
func (e *Engine) saveManifest(m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return e.transport.Put(manifestPath, strings.NewReader(string(data)))
}
