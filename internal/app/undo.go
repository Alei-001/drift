package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const operationsFile = "operations.log"

type OpType string

const (
	OpSave         OpType = "save"
	OpSwitch       OpType = "switch"
	OpBranchDelete OpType = "branch-delete"
	OpBranchRename OpType = "branch-rename"
	OpRestore      OpType = "restore"
	OpNameAdd      OpType = "name-add"
	OpNameDelete   OpType = "name-delete"
)

type OperationEntry struct {
	Timestamp  time.Time   `json:"timestamp"`
	Op         OpType      `json:"op"`
	Desc       string      `json:"desc"`
	RefChanges []RefChange `json:"ref_changes"`
}

type RefChange struct {
	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type UndoResult struct {
	Entry          OperationEntry
	RemainingCount int
}

func (a *App) recordOperation(op OpType, desc string, changes []RefChange) error {
	entry := OperationEntry{
		Timestamp:  time.Now(),
		Op:         op,
		Desc:       desc,
		RefChanges: changes,
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
			continue
		}
		entries = append(entries, entry)
	}

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

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
	var totalRestored int
	for i := 0; i < count; i++ {
		entry, restored, err := a.undoOne()
		if err != nil {
			if i == 0 {
				return nil, err
			}
			return &UndoResult{Entry: lastEntry, RemainingCount: count - i}, nil
		}
		lastEntry = *entry
		totalRestored += restored
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

	if err := a.removeLastOperation(); err != nil {
		errs = append(errs, fmt.Errorf("failed to remove operation from log: %w", err))
	}

	if len(errs) > 0 {
		return &last, len(last.RefChanges) - len(errs), errors.Join(errs...)
	}

	return &last, len(last.RefChanges), nil
}
