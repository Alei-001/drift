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
)

// WatchState summarizes the runtime state of a watch daemon.
type WatchState struct {
	StartTime       int64  `json:"start_time"`
	AutoSaves       int    `json:"auto_saves"`
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

	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
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

	killProcess(pid)

	os.Remove(pidPath)
	os.Remove(statePath)
	// The daemon may have been killed while holding the workspace lock.
	// Remove it best-effort so subsequent commands are not blocked until
	// the stale-lock timeout elapses. Errors are ignored: if another
	// operation now holds the lock, AcquireWorkspaceLock will re-create
	// it, and the worst case is a missed coordination cycle.
	os.Remove(filepath.Join(driftDir, "workspace.lock"))

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
func RunDaemonLoop(ctx context.Context, store storage.Storer, cwd string, interval int, keep int) {
	driftDir := filepath.Join(cwd, ".drift")
	pidPath := filepath.Join(driftDir, "watch.pid")
	statePath := filepath.Join(driftDir, "watch.state")

	state := &WatchState{
		StartTime: time.Now().Unix(),
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
		case <-ticker.C:
			// DetectChanges acquires+releases the workspace lock; if another
			// operation holds it, detection simply fails and we retry next
			// tick. This replaces the former IsWorkspaceLocked pre-check.
			changes, err := DetectChanges(ctx, store, cwd)
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
			_, err = createSnapshotInLock(ctx, store, cwd, msg, "drift", nil)
			ReleaseWorkspaceLock(cwd)
			if err != nil {
				continue
			}

			state.AutoSaves++
			state.LastSaveTime = time.Now().Unix()
			state.LastSaveChanges = fmt.Sprintf("+%d ~%d -%d",
				len(changes.Added), len(changes.Modified), len(changes.Deleted))
			state.LastError = ""
			writeState(statePath, state)

			if keep > 0 {
				pruned, _ := pruneAutoSnapshots(store, keep)
				state.Pruned += pruned
				writeState(statePath, state)
			}
		}
	}
}

// pruneAutoSnapshots deletes old auto-saved snapshots, keeping at most `keep`.
// It returns the number of snapshots deleted.
func pruneAutoSnapshots(store storage.Storer, keep int) (int, error) {
	ctx := context.Background()
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

	sort.Slice(autoSnaps, func(i, j int) bool {
		return autoSnaps[i].Timestamp > autoSnaps[j].Timestamp
	})

	deleted := 0
	for i := keep; i < len(autoSnaps); i++ {
		if err := store.DeleteSnapshot(ctx, autoSnaps[i].ID); err != nil {
			continue
		}
		deleted++
	}

	return deleted, nil
}

func writeState(path string, state *WatchState) {
	data, _ := json.Marshal(state)
	os.WriteFile(path, data, 0644)
}
