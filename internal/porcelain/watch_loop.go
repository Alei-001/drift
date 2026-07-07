package porcelain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
)

// WatchState summarizes the runtime state of a watch daemon.
type WatchState struct {
	StartTime       int64  `json:"start_time"`
	Interval        int    `json:"interval"`
	AutoSaves       int    `json:"auto_saves"`
	MaxSaves        int    `json:"max_saves"`
	LastSaveTime    int64  `json:"last_save_time"`
	LastSaveChanges string `json:"last_save_changes"`
	Pruned          int    `json:"pruned"`
	LastError       string `json:"last_error"`
	Paused          bool   `json:"paused"`
}

// RunDaemonLoop runs the watch daemon loop. It periodically detects workspace
// changes and creates auto-snapshots when changes are found. It prunes old
// auto-snapshots to keep at most `keep` entries.
func RunDaemonLoop(ctx context.Context, store storage.Storer, cwd string, interval int, keep int, cfg *core.CoreConfig) {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if interval <= 0 {
		slog.Warn("invalid interval", "interval", interval)
		return
	}
	if keep < 0 {
		slog.Warn("invalid keep", "keep", keep)
		return
	}
	driftDir := filepath.Join(cwd, ".drift")
	pidPath := filepath.Join(driftDir, "watch.pid")
	statePath := filepath.Join(driftDir, "watch.state")

	defer func() {
		if r := recover(); r != nil {
			slog.Error("daemon panic", "panic", r)
			// Best-effort cleanup after a panic: the daemon is dying, so
			// stale pid/state/lock files are harmless if removal fails.
			os.Remove(pidPath)
			os.Remove(statePath)
			removeLockIfOwned(filepath.Join(driftDir, "workspace.lock"), os.Getpid())
		}
	}()

	state := &WatchState{
		StartTime: time.Now().Unix(),
		Interval:  interval,
		MaxSaves:  keep,
	}
	writeState(statePath, state)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			// Best-effort cleanup: the daemon is exiting, so stale pid/state
			// files are harmless if removal fails (the next start reclaims them).
			os.Remove(pidPath)
			os.Remove(statePath)
			return
		case <-ctx.Done():
			// Best-effort cleanup: same rationale as sigChan — the daemon is
			// exiting and leftover files will be reclaimed on next start.
			os.Remove(pidPath)
			os.Remove(statePath)
			return
		case <-ticker.C:
			// Re-read state to pick up pause/resume toggles set by
			// external commands (drift watch pause/resume). The in-memory
			// state is refreshed so subsequent writeState calls preserve
			// the flag.
			if fileState, err := readState(statePath); err == nil {
				state.Paused = fileState.Paused
			}
			if state.Paused {
				continue
			}
			// DetectChanges acquires+releases the workspace lock; if another
			// operation holds it, detection simply fails and we retry next
			// tick. This replaces the former IsWorkspaceLocked pre-check.
			changes, err := DetectChanges(ctx, store, cwd, cfg)
			if err != nil {
				state.LastError = "detect: " + err.Error()
				writeStateMergingPause(statePath, state)
				continue
			}
			total := len(changes.Added) + len(changes.Modified) + len(changes.Deleted)
			if total == 0 {
				continue
			}

			// Acquire the lock for the save itself. If contention occurs,
			// record it and wait for the next period rather than blocking.
			if err := AcquireWorkspaceLock(cwd); err != nil {
				state.LastError = err.Error()
				writeStateMergingPause(statePath, state)
				continue
			}
			msg := AutoSavePrefix + " " + time.Now().Format("2006-01-02 15:04")
			_, err = createSnapshotInLock(ctx, store, cwd, msg, "drift", cfg)
			if err != nil {
				state.LastError = "save: " + err.Error()
				ReleaseWorkspaceLock(cwd)
				writeStateMergingPause(statePath, state)
				continue
			}

			state.AutoSaves++
			state.LastSaveTime = time.Now().Unix()
			state.LastSaveChanges = fmt.Sprintf("+%d ~%d -%d",
				len(changes.Added), len(changes.Modified), len(changes.Deleted))
			state.LastError = ""

			if keep > 0 {
				pruned, pruneErr := pruneAutoSnapshots(ctx, store, keep)
				state.Pruned += pruned
				if pruneErr != nil {
					// Record but do not abort: a prune failure shouldn't undo a
					// successful save. The error will be retried next cycle.
					state.LastError = "prune: " + pruneErr.Error()
				}
			}

			ReleaseWorkspaceLock(cwd)
			writeStateMergingPause(statePath, state)
		}
	}
}

// pruneAutoSnapshots deletes old auto-saved snapshots, keeping at most `keep`.
// It returns the number of snapshots deleted.
func pruneAutoSnapshots(ctx context.Context, store storage.Storer, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}

	snapshots, err := store.ListSnapshots(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("list snapshots: %w", err)
	}

	var autoSnaps []*core.SnapshotSummary
	for _, s := range snapshots {
		if strings.HasPrefix(s.Message, AutoSavePrefix) {
			autoSnaps = append(autoSnaps, s)
		}
	}

	if len(autoSnaps) <= keep {
		return 0, nil
	}

	// Compute reachable snapshots from all branch/tag tips to avoid
	// deleting snapshots that are still part of active history.
	reachable := make(map[core.SnapshotID]bool)
	// Collect roots: all heads/ and tags/ refs. If ListRefs fails, abort
	// pruning entirely — deleting without reachability data risks
	// severing the PrevID chain and corrupting branch history.
	refs, err := store.ListRefs(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("list refs for reachability check: %w", err)
	}
	var queue []core.SnapshotID
	for _, ref := range refs {
		if !ref.Target.IsZero() {
			queue = append(queue, core.SnapshotID{Hash: ref.Target})
		}
	}
	// BFS from roots following PrevID
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		sid := queue[0]
		queue = queue[1:]
		if reachable[sid] {
			continue
		}
		reachable[sid] = true
		if snap, err := store.GetSnapshot(ctx, sid); err == nil && snap.PrevID != nil {
			queue = append(queue, *snap.PrevID)
		}
	}

	sort.Slice(autoSnaps, func(i, j int) bool {
		return autoSnaps[i].Timestamp > autoSnaps[j].Timestamp
	})

	deleted := 0
	var firstErr error
	for i := keep; i < len(autoSnaps); i++ {
		if reachable[autoSnaps[i].ID] {
			continue // skip snapshots still reachable from branch/tag tips
		}
		if err := store.DeleteSnapshot(ctx, autoSnaps[i].ID); err != nil {
			// Record the first deletion error but keep trying remaining
			// snapshots so a single corrupted entry doesn't block cleanup.
			if firstErr == nil {
				firstErr = fmt.Errorf("delete snapshot %s: %w", autoSnaps[i].ID.Hash.String(), err)
			}
			continue
		}
		deleted++
	}

	return deleted, firstErr
}

func writeState(path string, state *WatchState) {
	data, err := json.Marshal(state)
	if err != nil {
		slog.Warn("marshal watch state", "error", err)
		return
	}
	if err := fsutil.WriteFileAtomic(path, data, 0644); err != nil {
		slog.Warn("write watch state", "path", path, "error", err)
	}
}

// writeStateMergingPause writes state to the state file after re-reading the
// existing file to preserve the Paused flag. This prevents the daemon loop
// from clobbering a Paused=true that PauseDaemon wrote concurrently during
// the (potentially long) detect+save window of a tick. If the state file
// cannot be read (e.g. it does not exist yet), the write proceeds as-is.
func writeStateMergingPause(path string, state *WatchState) {
	if latest, err := readState(path); err == nil && latest != nil {
		state.Paused = latest.Paused
	}
	writeState(path, state)
}

// readState reads and unmarshals the watch state file. Returns an error if
// the file is missing or cannot be parsed.
func readState(path string) (*WatchState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state WatchState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
