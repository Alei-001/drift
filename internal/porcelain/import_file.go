package porcelain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
	"github.com/Alei-001/drift/internal/util/pathutil"
)

// ImportFileFromBranch reconstructs a single file from the target branch's
// HEAD snapshot and writes it to the current workspace at the same relative
// path. The workspace lock is acquired for the duration. The index is NOT
// updated: the imported file appears as a new (added) file in 'drift status'
// and is captured by the next 'drift save', which is the intended workflow.
//
// This is a non-merge file-level cherry-pick: it does not touch any other
// workspace files, does not move HEAD, and does not create a snapshot.
// It is useful for bringing a single file from an experimental branch
// into the current branch without switching.
//
// Returns the imported FileEntry and an error. ErrBranchNotFound is
// returned when the branch does not exist; ErrSnapshotNotFound when the
// branch has no snapshots; ErrFileNotFound when the file is not present
// in the branch's HEAD snapshot.
func ImportFileFromBranch(ctx context.Context, store storage.Storer, workDir, branchName, filePath string, cfg *core.CoreConfig) (*core.FileEntry, error) {
	_ = cfg

	relPath, err := pathutil.RelToWorkDir(workDir, filePath)
	if err != nil {
		return nil, fmt.Errorf("invalid file path: %w", err)
	}

	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)

	ref, err := store.GetRef(ctx, "heads/"+branchName)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("branch %q: %w", branchName, ErrBranchNotFound)
		}
		return nil, fmt.Errorf("read branch ref: %w", err)
	}
	if ref.Target.IsZero() {
		return nil, fmt.Errorf("branch %q has no snapshots: %w", branchName, ErrSnapshotNotFound)
	}

	snap, err := store.GetSnapshot(ctx, core.SnapshotID{Hash: ref.Target})
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}

	var targetEntry *core.FileEntry
	for i := range snap.Files {
		if snap.Files[i].Path == relPath {
			targetEntry = &snap.Files[i]
			break
		}
	}
	if targetEntry == nil {
		return nil, fmt.Errorf("%w: %q in branch %q", ErrFileNotFound, relPath, branchName)
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workDir: %w", err)
	}

	safePath, err := resolveSecurePath(absWorkDir, relPath)
	if err != nil {
		return nil, fmt.Errorf("validate path: %w", err)
	}

	parentDir := filepath.Dir(safePath)
	if err := os.MkdirAll(parentDir, fsutil.DefaultDirPerm); err != nil {
		return nil, fmt.Errorf("create parent dir: %w", err)
	}

	perm := os.FileMode(targetEntry.Mode & 0o777)
	// Mask group/other write bits (umask 0o022 semantics) to prevent
	// malicious snapshots from creating world-writable files on import.
	perm &^= 0o022
	if perm == 0 {
		perm = fsutil.DefaultFilePerm
	}

	if err := writeFileFromChunks(ctx, store, safePath, targetEntry.Chunks, perm); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	if err := os.Chtimes(safePath, time.Unix(0, targetEntry.ModTime), time.Unix(0, targetEntry.ModTime)); err != nil {
		return nil, fmt.Errorf("set modtime: %w", err)
	}

	slog.Info("file imported", "branch", branchName, "file", relPath, "size", targetEntry.Size)

	return targetEntry, nil
}
