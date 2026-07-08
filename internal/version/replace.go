package version

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// replaceExecutable atomically replaces the currently running executable
// with the file at newPath. It uses a two-step rename strategy that works
// on both Unix and Windows:
//
//  1. Rename the current executable to "<exe>.old" (Windows cannot overwrite
//     a running binary, but it can rename it).
//  2. Rename newPath to the original executable path.
//  3. Best-effort remove "<exe>.old".
//
// On failure the original executable is restored by renaming ".old" back,
// so the binary remains runnable.
func replaceExecutable(newPath string) error {
	exePath, err := currentExecutablePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exePath)

	// Ensure the new binary is on the same filesystem as the target so that
	// os.Rename works (rename across filesystems fails on Unix). When the
	// caller passed a temp file on a different volume, copy it in place.
	if !sameFilesystem(newPath, exePath) {
		copied, err := copyToDir(newPath, dir)
		if err != nil {
			return err
		}
		defer os.Remove(copied)
		newPath = copied
	}

	// Make the new binary executable (Unix); harmless on Windows.
	_ = os.Chmod(newPath, 0755)

	oldPath := exePath + ".old"
	// Remove a stale .old from a previous failed upgrade, if any.
	_ = os.Remove(oldPath)

	// Step 1: move the running binary aside.
	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("rename current binary: %w", err)
	}

	// Step 2: move the new binary into place.
	if err := os.Rename(newPath, exePath); err != nil {
		// Roll back: restore the original.
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("install new binary: %w", err)
	}

	// Step 3: best-effort remove the old binary. On Windows the running
	// process still holds a handle to the renamed file, so deletion may
	// fail; that's fine — the file becomes removable after exit. We leave
	// it and the next upgrade cleans it up.
	if err := os.Remove(oldPath); err != nil {
		// Non-fatal: report via a sentinel so the CLI can hint.
		return errOldBinaryRemovableLater
	}
	return nil
}

// errOldBinaryRemovableLater signals that the upgrade succeeded but the
// previous binary (renamed to .old) could not be deleted immediately. The
// CLI treats this as success with a warning.
var errOldBinaryRemovableLater = errors.New("old binary retained, will be removed on next run")

// currentExecutablePath returns the absolute, symlink-resolved path to the
// running executable.
func currentExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Resolve symlinks so we replace the real file, not a link to it.
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		// Fall back to the unresolved path; common on some Windows setups.
		return exe, nil
	}
	return resolved, nil
}

// sameFilesystem reports whether two paths are likely on the same filesystem.
// Because cross-device rename fails on Unix, we conservatively assume paths
// under the same parent directory are co-located; otherwise we copy. This is
// a heuristic — correctness is ensured by the copy fallback when in doubt.
func sameFilesystem(a, b string) bool {
	ra, erra := filepath.EvalSymlinks(a)
	rb, errb := filepath.EvalSymlinks(b)
	if erra != nil || errb != nil {
		return false
	}
	return filepath.Dir(ra) == filepath.Dir(rb)
}

// copyToDir copies src into dir with a temp name and returns the new path.
func copyToDir(src, dir string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	dst, err := os.CreateTemp(dir, "drift-staging-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(dst, in); err != nil {
		dst.Close()
		os.Remove(dst.Name())
		return "", err
	}
	dst.Close()
	return dst.Name(), nil
}
