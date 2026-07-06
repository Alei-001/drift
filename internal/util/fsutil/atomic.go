package fsutil

import (
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path atomically: it writes to a temp file
// in the same directory, fsyncs and closes it, applies perm, renames it over
// the target, and fsyncs the parent directory. On any error the temp file is
// removed and the target is left untouched. The temp file is closed before
// the rename so the rename succeeds on platforms (e.g. Windows) that reject
// renaming an open file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	prefix := filepath.Base(path) + ".tmp"

	f, err := os.CreateTemp(dir, prefix)
	if err != nil {
		return err
	}
	tmpPath := f.Name()

	if _, err := f.Write(data); err != nil {
		// Best-effort: temp file may already be removed.
		_ = f.Close()
		// Best-effort: temp file may already be removed.
		_ = os.Remove(tmpPath)
		return err
	}

	if err := f.Sync(); err != nil {
		// Best-effort: temp file may already be removed.
		_ = f.Close()
		// Best-effort: temp file may already be removed.
		_ = os.Remove(tmpPath)
		return err
	}

	if err := f.Close(); err != nil {
		// Best-effort: temp file may already be removed.
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Chmod(tmpPath, perm); err != nil {
		// Best-effort: temp file may already be removed.
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Best-effort: temp file may already be removed.
		_ = os.Remove(tmpPath)
		return err
	}

	// fsync parent directory to ensure the rename is persisted to disk.
	// Best-effort: on platforms where opening a directory fails (e.g.
	// Windows), the error is silently ignored.
	if d, err := os.Open(dir); err == nil {
		// Best-effort: dir may not support sync (Windows).
		_ = d.Sync()
		// Best-effort: dir handle may already be closed.
		_ = d.Close()
	}

	return nil
}
