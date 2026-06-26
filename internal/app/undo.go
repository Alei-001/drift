package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/drift/drift/internal/core"
)

const operationsFile = "operations.log"

type OpType string

const (
	OpSave         OpType = "save"
	OpSwitch       OpType = "switch"
	OpBranchCreate OpType = "branch-create"
	OpBranchDelete OpType = "branch-delete"
	OpBranchRename OpType = "branch-rename"
	OpRestore      OpType = "restore"
	OpTagAdd      OpType = "tag-add"
	OpTagDelete   OpType = "tag-delete"
)

type OperationEntry struct {
	Timestamp     time.Time        `json:"timestamp"`
	Op            OpType           `json:"op"`
	Desc          string           `json:"desc"`
	RefChanges    []RefChange      `json:"ref_changes"`
	IndexSnapshot []core.IndexEntry `json:"index_snapshot,omitempty"`
}

type RefChange struct {
	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type UndoResult struct {
	Entry          OperationEntry
	RemainingCount int
	Warning        string
}

func (a *App) recordOperation(op OpType, desc string, changes []RefChange) error {
	return a.recordOperationWithIndex(op, desc, changes, nil)
}

func (a *App) recordOperationWithIndex(op OpType, desc string, changes []RefChange, indexSnapshot []core.IndexEntry) error {
	entry := OperationEntry{
		Timestamp:     time.Now(),
		Op:            op,
		Desc:          desc,
		RefChanges:    changes,
		IndexSnapshot: indexSnapshot,
	}

	path := filepath.Join(a.store.DriftDir(), operationsFile)
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return a.store.WithLock(func() error {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.Write(data)
		return err
	})
}

func (a *App) ReadOperations() ([]OperationEntry, error) {
	path := filepath.Join(a.store.DriftDir(), operationsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []OperationEntry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry OperationEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping malformed operation log entry: %v\n", err)
			continue
		}
		entries = append(entries, entry)
	}

	slices.Reverse(entries)

	return entries, nil
}

func (a *App) removeLastOperation() error {
	path := filepath.Join(a.store.DriftDir(), operationsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	lines = lines[:len(lines)-1]
	out := strings.Join(lines, "\n")
	if len(lines) > 0 {
		out += "\n"
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *App) Undo(count int) (*UndoResult, error) {
	if count < 1 {
		return nil, fmt.Errorf("undo count must be positive")
	}

	var lastEntry OperationEntry
	for i := 0; i < count; i++ {
		entry, _, err := a.undoOne()
		if err != nil {
			if i == 0 {
				return nil, err
			}
			return &UndoResult{
				Entry:          lastEntry,
				RemainingCount: count - i,
				Warning:        err.Error(),
			}, nil
		}
		lastEntry = *entry
	}

	return &UndoResult{Entry: lastEntry, RemainingCount: 0}, nil
}

func (a *App) undoOne() (*OperationEntry, int, error) {
	entries, err := a.ReadOperations()
	if err != nil {
		return nil, 0, err
	}
	if len(entries) == 0 {
		return nil, 0, fmt.Errorf("no operations to undo")
	}

	last := entries[0]

	var errs []error
	for _, change := range last.RefChanges {
		if change.Before == "" {
			if err := a.store.DeleteRef(change.Ref); err != nil {
				errs = append(errs, fmt.Errorf("failed to delete ref %s: %w", change.Ref, err))
			}
		} else {
			if err := a.store.SaveRef(change.Ref, change.Before); err != nil {
				errs = append(errs, fmt.Errorf("failed to restore ref %s: %w", change.Ref, err))
			}
		}
	}

	// Restore index snapshot if present (e.g. for restore operations).
	if len(last.IndexSnapshot) > 0 {
		oldIdx := &core.Index{}
		for _, e := range last.IndexSnapshot {
			oldIdx.Add(e)
		}
		if err := a.store.SaveIndex(oldIdx); err != nil {
			errs = append(errs, fmt.Errorf("failed to restore index: %w", err))
		}
	}

	// Only remove the operation from the log if all ref changes succeeded.
	// This allows the user to retry undo if a ref change failed.
	if len(errs) > 0 {
		return &last, len(last.RefChanges) - len(errs), errors.Join(errs...)
	}

	if err := a.removeLastOperation(); err != nil {
		return &last, 0, fmt.Errorf("failed to remove operation from log: %w", err)
	}

	return &last, len(last.RefChanges), nil
}
