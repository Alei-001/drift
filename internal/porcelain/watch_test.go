package porcelain

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

func setupTestStore(t *testing.T) *memory.MemoryStorage {
	t.Helper()
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	store.SetIndex(context.Background(), &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	})
	return store
}

func TestPruneAutoSnapshots(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, "file.txt"), []byte(strings.Repeat("x", i+1)), 0644)
		_, err := CreateSnapshot(context.Background(), store, dir, fmt.Sprintf("auto - snapshot %d", i), "drift", nil)
		if err != nil {
			t.Fatalf("auto CreateSnapshot %d failed: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		os.WriteFile(filepath.Join(dir, "file.txt"), []byte(strings.Repeat("y", i+10)), 0644)
		_, err := CreateSnapshot(context.Background(), store, dir, fmt.Sprintf("manual %d", i), "test", nil)
		if err != nil {
			t.Fatalf("manual CreateSnapshot %d failed: %v", i, err)
		}
	}

	deleted, err := pruneAutoSnapshots(context.Background(), store, 3)
	if err != nil {
		t.Fatalf("pruneAutoSnapshots failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}

	snaps, err := store.ListSnapshots(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	autoCount := 0
	manualCount := 0
	for _, s := range snaps {
		if strings.HasPrefix(s.Message, "auto -") {
			autoCount++
		} else {
			manualCount++
		}
	}
	if autoCount != 5 {
		t.Errorf("expected 5 auto snapshots remaining, got %d", autoCount)
	}
	if manualCount != 2 {
		t.Errorf("expected 2 manual snapshots remaining, got %d", manualCount)
	}
}

func TestPruneAutoSnapshots_NothingToPrune(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir()

	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(dir, "file.txt"), []byte(strings.Repeat("x", i+1)), 0644)
		CreateSnapshot(context.Background(), store, dir, fmt.Sprintf("auto - %d", i), "drift", nil)
	}

	deleted, err := pruneAutoSnapshots(context.Background(), store, 5)
	if err != nil {
		t.Fatalf("pruneAutoSnapshots failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted, got %d", deleted)
	}
}

func TestWatchState_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, ".drift", "watch.state")
	os.MkdirAll(filepath.Dir(statePath), 0755)

	original := &WatchState{
		StartTime:       time.Now().Unix(),
		AutoSaves:       5,
		LastSaveTime:    time.Now().Add(-time.Hour).Unix(),
		LastSaveChanges: "+2 ~1",
		Pruned:          3,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	readData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	var loaded WatchState
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if loaded.StartTime != original.StartTime {
		t.Errorf("StartTime mismatch: got %d, want %d", loaded.StartTime, original.StartTime)
	}
	if loaded.AutoSaves != original.AutoSaves {
		t.Errorf("AutoSaves mismatch: got %d, want %d", loaded.AutoSaves, original.AutoSaves)
	}
	if loaded.LastSaveTime != original.LastSaveTime {
		t.Errorf("LastSaveTime mismatch: got %d, want %d", loaded.LastSaveTime, original.LastSaveTime)
	}
	if loaded.LastSaveChanges != original.LastSaveChanges {
		t.Errorf("LastSaveChanges mismatch: got %s, want %s", loaded.LastSaveChanges, original.LastSaveChanges)
	}
	if loaded.Pruned != original.Pruned {
		t.Errorf("Pruned mismatch: got %d, want %d", loaded.Pruned, original.Pruned)
	}
}

// TestStartDaemon_AlreadyRunning verifies the pid-file guard: when a live
// daemon pid is already recorded, StartDaemon refuses to spawn a second one.
// This exercises StartDaemon's pre-spawn logic without launching a subprocess.
func TestStartDaemon_AlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	driftDir := filepath.Join(dir, ".drift")
	if err := os.MkdirAll(driftDir, 0755); err != nil {
		t.Fatalf("mkdir .drift: %v", err)
	}
	// Pre-write a pid file pointing at the current (live) test process.
	pidPath := filepath.Join(driftDir, "watch.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	_, err := StartDaemon(context.Background(), dir, 1, 1)
	if err == nil {
		t.Fatal("expected error when a daemon is already running, got nil")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("expected 'already running' in error, got %v", err)
	}
}

// TestStartDaemon_ReclaimsStalePidFile verifies the O_EXCL pid-file path:
// when the existing pid file points at a dead process, StartDaemon's pre-check
// removes it and the O_EXCL create succeeds for the freshly spawned daemon.
// This is the regression test for the TOCTOU fix that replaced the rename-based
// WriteFileAtomic (which would silently clobber a concurrently-created pid
// file) with O_CREATE|O_EXCL.
func TestStartDaemon_ReclaimsStalePidFile(t *testing.T) {
	if os.Getenv("DRIFT_TEST_DAEMON_GUARD") != "" {
		t.Skip("recursive invocation from StartDaemon subprocess")
	}
	t.Setenv("DRIFT_TEST_DAEMON_GUARD", "1")

	dir := t.TempDir()
	driftDir := filepath.Join(dir, ".drift")
	if err := os.MkdirAll(driftDir, 0755); err != nil {
		t.Fatalf("mkdir .drift: %v", err)
	}
	// Pre-write a pid file pointing at a dead process. StartDaemon's pre-check
	// should detect it as stale, remove it, and proceed to O_EXCL-create a new
	// pid file for the spawned daemon.
	pidPath := filepath.Join(driftDir, "watch.pid")
	if err := os.WriteFile(pidPath, []byte("999999"), 0644); err != nil {
		t.Fatalf("write stale pid file: %v", err)
	}

	pid, err := StartDaemon(context.Background(), dir, 1, 1)
	if err != nil {
		t.Fatalf("StartDaemon should reclaim a stale pid file: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("expected positive pid, got %d", pid)
	}

	// The pid file must now contain the new daemon's pid, not the stale one.
	got, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	if strings.TrimSpace(string(got)) != fmt.Sprintf("%d", pid) {
		t.Errorf("pid file = %s, want %d", string(got), pid)
	}

	t.Cleanup(func() {
		if err := killProcess(pid); err != nil {
			t.Logf("kill daemon pid %d: %v (may have already exited)", pid, err)
		}
		os.Remove(pidPath)
	})
}

// TestStartDaemon_IgnoresArgs0 is the regression test for the PATH-hijack
// vulnerability: StartDaemon must resolve the daemon binary via os.Executable()
// and must NOT trust os.Args[0]. Sabotaging os.Args[0] to a nonexistent path
// would make the old code fail at cmd.Start with "executable file not found";
// with the fix StartDaemon still launches the real binary and returns a pid.
func TestStartDaemon_IgnoresArgs0(t *testing.T) {
	// The daemon subprocess is the test binary itself, which re-runs this
	// package's tests when spawned. The sentinel (inherited via the env)
	// makes the recursive invocation skip instead of spawning another daemon.
	if os.Getenv("DRIFT_TEST_DAEMON_GUARD") != "" {
		t.Skip("recursive invocation from StartDaemon subprocess")
	}
	t.Setenv("DRIFT_TEST_DAEMON_GUARD", "1")

	dir := t.TempDir()
	// StartDaemon writes watch.pid into .drift/ but does not create that
	// directory itself (project init does). Pre-create it so the full
	// success path completes and we get the pid back for cleanup.
	if err := os.MkdirAll(filepath.Join(dir, ".drift"), 0755); err != nil {
		t.Fatalf("mkdir .drift: %v", err)
	}

	origArg0 := os.Args[0]
	os.Args[0] = filepath.Join(t.TempDir(), "nonexistent-drift-binary")
	t.Cleanup(func() { os.Args[0] = origArg0 })

	pid, err := StartDaemon(context.Background(), dir, 1, 1)
	if err != nil {
		t.Fatalf("StartDaemon should ignore os.Args[0] and resolve via os.Executable(): %v", err)
	}
	if pid <= 0 {
		t.Fatalf("expected positive pid, got %d", pid)
	}

	// Reap the spawned subprocess and clean up its pid file. Always run
	// cleanup so a failed assertion does not orphan the daemon process.
	t.Cleanup(func() {
		if err := killProcess(pid); err != nil {
			t.Logf("kill daemon pid %d: %v (may have already exited)", pid, err)
		}
		os.Remove(filepath.Join(dir, ".drift", "watch.pid"))
	})
}
