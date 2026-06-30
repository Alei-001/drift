package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/cache"
)

// FSStorage implements storage.Storer using the local filesystem.
type FSStorage struct {
	root         string
	chunkCache   *cache.Cache[core.Hash, *core.Chunk]
	previewCache *cache.Cache[string, []byte]
	zstdDecoder  *zstd.Decoder
	zstdEncoder  *zstd.Encoder
	// storageLockFile holds the open handle to the storage lock file for the
	// lifetime of the storage. Its existence on disk (created with O_EXCL)
	// is what actually serializes concurrent drift processes; keeping the
	// handle open lets Close reliably release it.
	storageLockFile *os.File
	// storageLockPath is the absolute path of the lock file, cached so Close
	// can remove it even if the handle has already been closed.
	storageLockPath string
}

// NewFSStorage creates or opens a filesystem-backed storage at root (the .drift/ directory).
func NewFSStorage(root string) (*FSStorage, error) {
	dirs := []string{
		filepath.Join(root, ChunksDir),
		filepath.Join(root, SnapshotsDir),
		filepath.Join(root, RefsDir),
		filepath.Join(root, PreviewsDir),
		filepath.Join(root, RefsDir, HeadsDir),
		filepath.Join(root, RefsDir, TagsDir),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, err
		}
	}

	// Acquire the process-level storage lock before any further
	// initialization so two concurrent drift processes cannot observe a
	// half-initialized store.
	lockPath := filepath.Join(root, StorageLockFile)
	lockFile, err := acquireStorageLock(lockPath)
	if err != nil {
		return nil, err
	}

	chunkCache, err := cache.NewCache[core.Hash, *core.Chunk](256)
	if err != nil {
		releaseStorageLock(lockFile, lockPath)
		return nil, err
	}
	previewCache, err := cache.NewCache[string, []byte](64)
	if err != nil {
		releaseStorageLock(lockFile, lockPath)
		return nil, err
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		releaseStorageLock(lockFile, lockPath)
		return nil, err
	}
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		decoder.Close()
		releaseStorageLock(lockFile, lockPath)
		return nil, err
	}

	return &FSStorage{
		root:            root,
		chunkCache:      chunkCache,
		previewCache:    previewCache,
		zstdDecoder:     decoder,
		zstdEncoder:     encoder,
		storageLockFile: lockFile,
		storageLockPath: lockPath,
	}, nil
}

// Close releases resources held by the storage, including the storage lock.
func (fs *FSStorage) Close() error {
	fs.zstdDecoder.Close()
	encErr := fs.zstdEncoder.Close()
	releaseStorageLock(fs.storageLockFile, fs.storageLockPath)
	fs.storageLockFile = nil
	fs.storageLockPath = ""
	return encErr
}

// acquireStorageLock creates an exclusive lock file at lockPath containing the
// current process PID and returns the open file handle. The handle must be
// kept open for the lifetime of the storage so other drift processes see the
// lock; releaseStorageLock closes and removes it.
//
// If a lock file already exists, the holder's PID is inspected: a dead holder
// is treated as stale (its lock file is removed and the acquisition retried
// once), while a live holder causes the function to return an error wrapping
// storage.ErrLocked.
func acquireStorageLock(lockPath string) (*os.File, error) {
	pidStr := strconv.Itoa(os.Getpid())

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err == nil {
		return writeLockPID(f, lockPath, pidStr)
	}
	if !os.IsExist(err) {
		return nil, fmt.Errorf("create storage lock: %w", err)
	}

	// Lock file already exists — inspect the holder.
	data, rerr := os.ReadFile(lockPath)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			// Raced with another process releasing the lock; retry once.
			return retryStorageLock(lockPath, pidStr)
		}
		return nil, fmt.Errorf("read storage lock: %w", rerr)
	}

	holderPID, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil || holderPID <= 0 {
		// Unreadable / corrupt lock — treat as stale and retry once.
		if rmErr := os.Remove(lockPath); rmErr != nil {
			return nil, fmt.Errorf("remove corrupt storage lock: %w", rmErr)
		}
		return retryStorageLock(lockPath, pidStr)
	}

	if processExists(holderPID) {
		return nil, fmt.Errorf("storage is locked by PID %d: %w", holderPID, storage.ErrLocked)
	}

	// Holder is dead — remove the stale lock and retry once.
	if rmErr := os.Remove(lockPath); rmErr != nil {
		return nil, fmt.Errorf("remove stale storage lock: %w", rmErr)
	}
	return retryStorageLock(lockPath, pidStr)
}

// retryStorageLock attempts a single O_CREATE|O_EXCL acquisition after a stale
// lock has been removed. It is a best-effort retry: if another process raced
// ahead and re-acquired the lock, an error wrapping storage.ErrLocked is
// returned.
func retryStorageLock(lockPath, pidStr string) (*os.File, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("storage is locked: %w", storage.ErrLocked)
		}
		return nil, fmt.Errorf("create storage lock: %w", err)
	}
	return writeLockPID(f, lockPath, pidStr)
}

// writeLockPID writes the PID string to the lock file and returns the handle.
// On write failure the file is closed and removed so a half-written lock does
// not block subsequent attempts.
func writeLockPID(f *os.File, lockPath, pidStr string) (*os.File, error) {
	if _, err := f.Write([]byte(pidStr)); err != nil {
		f.Close()
		os.Remove(lockPath)
		return nil, fmt.Errorf("write storage lock: %w", err)
	}
	return f, nil
}

// releaseStorageLock closes the lock handle (if non-nil) and removes the lock
// file (if non-empty). Failures are intentionally ignored: Close must be
// best-effort and should not mask the encoder error returned by FSStorage.Close.
func releaseStorageLock(f *os.File, path string) {
	if f != nil {
		f.Close()
	}
	if path != "" {
		os.Remove(path)
	}
}
