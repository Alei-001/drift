package porcelain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

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
		// Best-effort: the pid file points at a dead process; removal failure
		// is harmless since O_EXCL create below will detect the stale file.
		os.Remove(pidPath)
	}

	// Resolve the current executable path rather than trusting os.Args[0].
	// A malicious drift binary placed earlier on PATH could otherwise hijack
	// the daemon subprocess, since os.Args[0] is whatever the parent shell
	// used to launch us and need not be an absolute path.
	exePath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("resolve executable path: %w", err)
	}
	cmd := exec.Command(exePath, "_watch_daemon",
		"--interval", strconv.Itoa(interval),
		"--keep", strconv.Itoa(keep))
	cmd.Dir = cwd
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}

	pid := cmd.Process.Pid

	// Create the pid file atomically with O_CREATE|O_EXCL. If another daemon
	// won the race to create the pid file between our pre-check and cmd.Start,
	// we must not overwrite its pid file — instead kill the subprocess we just
	// spawned and report the failure. This closes the TOCTOU window that
	// existed when the pid file was written with a rename-based atomic write
	// (which silently clobbers an existing file).
	f, err := os.OpenFile(pidPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		// Best-effort: the spawned daemon cannot be used without a pid file;
		// kill it so it does not linger. A failed kill means it already exited.
		cmd.Process.Kill()
		return 0, fmt.Errorf("write pid file: %w", err)
	}
	if _, err := f.Write([]byte(strconv.Itoa(pid))); err != nil {
		// Best-effort cleanup: close the half-open file, remove the incomplete
		// pid file, and kill the spawned daemon. None of these can usefully
		// fail — the caller will retry from scratch on the next invocation.
		f.Close()
		os.Remove(pidPath)
		cmd.Process.Kill()
		return 0, fmt.Errorf("write pid file: %w", err)
	}
	if err := f.Close(); err != nil {
		// Best-effort cleanup: same rationale as the write-error path above.
		os.Remove(pidPath)
		cmd.Process.Kill()
		return 0, fmt.Errorf("close pid file: %w", err)
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
		slog.Warn("kill daemon failed", "pid", pid, "error", err)
	}

	// Best-effort cleanup: the daemon is being stopped, so stale pid/state
	// files are harmless if removal fails (the next start reclaims them).
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

// PauseDaemon marks the running daemon as paused by setting the Paused flag
// in the watch state file. The daemon loop re-reads this flag at the start
// of each tick and skips detection while paused. Returns an error if no
// daemon is running or the daemon is already paused.
func PauseDaemon(ctx context.Context, cwd string) (*WatchState, error) {
	_, active, err := DaemonStatus(ctx, cwd)
	if err != nil {
		return nil, fmt.Errorf("check daemon status: %w", err)
	}
	if !active {
		return nil, fmt.Errorf("daemon is not running (or already paused)")
	}
	statePath := filepath.Join(cwd, ".drift", "watch.state")
	state, err := readState(statePath)
	if err != nil || state == nil {
		// State file not yet written (daemon just started); create one
		// with Paused=true so the daemon picks it up on the next tick.
		state = &WatchState{Paused: true}
	} else if state.Paused {
		return nil, fmt.Errorf("daemon is not running (or already paused)")
	} else {
		state.Paused = true
	}
	writeState(statePath, state)
	return state, nil
}

// ResumeDaemon clears the Paused flag in the watch state file so the
// running daemon resumes detection on its next tick. Returns an error if
// no daemon is running or the daemon is not paused.
func ResumeDaemon(ctx context.Context, cwd string) (*WatchState, error) {
	_, active, err := DaemonStatus(ctx, cwd)
	if err != nil {
		return nil, fmt.Errorf("check daemon status: %w", err)
	}
	if !active {
		return nil, fmt.Errorf("daemon is not running")
	}
	statePath := filepath.Join(cwd, ".drift", "watch.state")
	state, err := readState(statePath)
	if err != nil || state == nil {
		return nil, fmt.Errorf("daemon is not paused")
	}
	if !state.Paused {
		return nil, fmt.Errorf("daemon is not paused")
	}
	state.Paused = false
	writeState(statePath, state)
	return state, nil
}
