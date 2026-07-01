package porcelain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
)

// WatchState summarizes the runtime state of a watch daemon.
type WatchState struct {
	StartTime       int64  `json:"start_time"`
	AutoSaves       int    `json:"auto_saves"`
	MaxSaves        int    `json:"max_saves"`
	LastSaveTime    int64  `json:"last_save_time"`
	LastSaveChanges string `json:"last_save_changes"`
	Pruned          int    `json:"pruned"`
	LastError       string `json:"last_error"`
}

// StartDaemon starts a background watch daemon for the project at cwd.
// It returns the PID of the started process.
func StartDaemon(ctx context.Context, cwd string, interval int, keep int) (int, error) {
	driftDir := filepath.Join(cwd, ".drift")
	pidPath := filepath.Join(driftDir, "watch.pid")

	if data, err := os.ReadFile(pidPath); err == nil {
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && processExists(pid) {
			return 0, fmt.Errorf("a watch daemon is already running (PID %d)", pid)
		}
		os.Remove(pidPath)
	}

	cmd := exec.Command(os.Args[0], "_watch_daemon",
		"--interval", strconv.Itoa(interval),
		"--keep", strconv.Itoa(keep))
	cmd.Dir = cwd
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}

	pid := cmd.Process.Pid

	if err := fsutil.WriteFileAtomic(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return 0, fmt.Errorf("write pid file: %w", err)
	}

	return pid, nil
}

// StopDaemon stops the watch daemon for the project at cwd.
// Returns the number of auto-saves created and snapshots pruned during the session.
func StopDaemon(ctx context.Context, cwd string) (int, int, error) {
	driftDir := filepath.Join(cwd, ".drift")
	pidPath := filepath.Join(driftDir, "watch.pid")
	statePath := filepath.Join(driftDir, "watch.state")

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, 0, fmt.Errorf("no watch daemon running")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid PID file")
	}

	autoSaves := 0
	pruned := 0
	if stateData, err := os.ReadFile(statePath); err == nil {
		var state WatchState
		if err := json.Unmarshal(stateData, &state); err == nil {
			autoSaves = state.AutoSaves
			pruned = state.Pruned
		}
	}

	if err := killProcess(pid); err != nil {
		// Log but don't fail — the pid file is still cleaned up below,
		// and a failed kill usually means the process already exited.
		fmt.Fprintf(os.Stderr, "warning: kill daemon PID %d: %v\n", pid, err)
	}

	os.Remove(pidPath)
	os.Remove(statePath)
	// The daemon may have been killed while holding the workspace lock.
	// Only remove the lock if it still belongs to the daemon's PID, so we
	// don't clobber a lock acquired by another command in the meantime.
	removeLockIfOwned(filepath.Join(driftDir, "workspace.lock"), pid)

	return autoSaves, pruned, nil
}

// DaemonStatus checks whether a watch daemon is running for the project at cwd.
// If the daemon is alive, it returns the state and true.
// If the daemon is not running, it cleans up stale files and returns nil, false, nil.
func DaemonStatus(ctx context.Context, cwd string) (*WatchState, bool, error) {
	driftDir := filepath.Join(cwd, ".drift")
	pidPath := filepath.Join(driftDir, "watch.pid")
	statePath := filepath.Join(driftDir, "watch.state")

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return nil, false, nil
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		os.Remove(pidPath)
		return nil, false, nil
	}

	if !processExists(pid) {
		os.Remove(pidPath)
		os.Remove(statePath)
		return nil, false, nil
	}

	stateData, err := os.ReadFile(statePath)
	if err != nil {
		return nil, true, nil
	}
	var state WatchState
	if err := json.Unmarshal(stateData, &state); err != nil {
		return nil, true, nil
	}
	return &state, true, nil
}

// RunDaemonLoop runs the watch daemon loop. It periodically detects workspace
// changes and creates auto-snapshots when changes are found. It prunes old
// auto-snapshots to keep at most `keep` entries.
func RunDaemonLoop(ctx context.Context, store storage.Storer, cwd string, interval int, keep int, cfg *core.CoreConfig) {
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}
	if interval <= 0 {
		fmt.Fprintf(os.Stderr, "invalid interval: %d (must be > 0)\n", interval)
		return
	}
	if keep < 0 {
		fmt.Fprintf(os.Stderr, "invalid keep: %d (must be >= 0)\n", keep)
		return
	}
	driftDir := filepath.Join(cwd, ".drift")
	pidPath := filepath.Join(driftDir, "watch.pid")
	statePath := filepath.Join(driftDir, "watch.state")

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "daemon panic: %v\n", r)
			os.Remove(pidPath)
			os.Remove(statePath)
			removeLockIfOwned(filepath.Join(driftDir, "workspace.lock"), os.Getpid())
		}
	}()

	state := &WatchState{
		StartTime: time.Now().Unix(),
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
			os.Remove(pidPath)
			os.Remove(statePath)
			return
		case <-ctx.Done():
			os.Remove(pidPath)
			os.Remove(statePath)
			return
		case <-ticker.C:
			// DetectChanges acquires+releases the workspace lock; if another
			// operation holds it, detection simply fails and we retry next
			// tick. This replaces the former IsWorkspaceLocked pre-check.
			changes, err := DetectChanges(ctx, store, cwd, cfg)
			if err != nil {
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
				writeState(statePath, state)
				continue
			}
			msg := "auto - " + time.Now().Format("2006-01-02 15:04")
			_, err = createSnapshotInLock(ctx, store, cwd, msg, "drift", nil, cfg)
			if err != nil {
				ReleaseWorkspaceLock(cwd)
				continue
			}

			state.AutoSaves++
			state.LastSaveTime = time.Now().Unix()
			state.LastSaveChanges = fmt.Sprintf("+%d ~%d -%d",
				len(changes.Added), len(changes.Modified), len(changes.Deleted))
			state.LastError = ""

			if keep > 0 {
				pruned, _ := pruneAutoSnapshots(ctx, store, keep)
				state.Pruned += pruned
			}

			ReleaseWorkspaceLock(cwd)
			writeState(statePath, state)
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
		return 0, err
	}

	var autoSnaps []*core.Snapshot
	for _, s := range snapshots {
		if strings.HasPrefix(s.Message, "auto -") {
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
	for i := keep; i < len(autoSnaps); i++ {
		if reachable[autoSnaps[i].ID] {
			continue // skip snapshots still reachable from branch/tag tips
		}
		if err := store.DeleteSnapshot(ctx, autoSnaps[i].ID); err != nil {
			continue
		}
		deleted++
	}

	return deleted, nil
}

// removeLockIfOwned removes the workspace lock file only if it was created by
// the given PID. This prevents clobbering a lock that another command has
// acquired after the daemon was killed.
func removeLockIfOwned(lockPath string, pid int) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return
	}
	var lock workspaceLockData
	if json.Unmarshal(data, &lock) != nil {
		return
	}
	if lock.PID == pid {
		os.Remove(lockPath)
	}
}

func writeState(path string, state *WatchState) {
	data, err := json.Marshal(state)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal watch state: %v\n", err)
		return
	}
	if err := fsutil.WriteFileAtomic(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write watch state: %v\n", err)
	}
}
