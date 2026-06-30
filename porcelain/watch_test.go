package porcelain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"context"
	"testing"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage/memory"
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
		_, err := CreateSnapshot(store, dir, fmt.Sprintf("auto - snapshot %d", i), "drift", nil)
		if err != nil {
			t.Fatalf("auto CreateSnapshot %d failed: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		os.WriteFile(filepath.Join(dir, "file.txt"), []byte(strings.Repeat("y", i+10)), 0644)
		_, err := CreateSnapshot(store, dir, fmt.Sprintf("manual %d", i), "test", nil)
		if err != nil {
			t.Fatalf("manual CreateSnapshot %d failed: %v", i, err)
		}
	}

	deleted, err := pruneAutoSnapshots(store, 3)
	if err != nil {
		t.Fatalf("pruneAutoSnapshots failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
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
	if autoCount != 3 {
		t.Errorf("expected 3 auto snapshots remaining, got %d", autoCount)
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
		CreateSnapshot(store, dir, fmt.Sprintf("auto - %d", i), "drift", nil)
	}

	deleted, err := pruneAutoSnapshots(store, 5)
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
