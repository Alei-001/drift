package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const operationsFile = "operations.log"

// OpType enumerates operation types recorded in the history.
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

// OperationEntry records a single state-changing operation.
type OperationEntry struct {
	Timestamp  time.Time   `json:"timestamp"`
	Op         OpType      `json:"op"`
	Desc       string      `json:"desc"`
	RefChanges []RefChange `json:"ref_changes"`
}

// RefChange records the before/after value of a single ref.
type RefChange struct {
	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// RecordOperation appends an operation entry to the operations log.
func (r *Repository) RecordOperation(op OpType, desc string, changes []RefChange) {
	entry := OperationEntry{
		Timestamp:  time.Now(),
		Op:         op,
		Desc:       desc,
		RefChanges: changes,
	}

	path := filepath.Join(r.Store.DriftDir(), operationsFile)
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}

// ReadOperations reads all operations from the log, newest first.
func (r *Repository) ReadOperations() ([]OperationEntry, error) {
	path := filepath.Join(r.Store.DriftDir(), operationsFile)
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

	// Reverse to newest-first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return entries, nil
}

// RemoveLastOperation removes the last (most recent) entry from the operations log.
func (r *Repository) RemoveLastOperation() error {
	path := filepath.Join(r.Store.DriftDir(), operationsFile)
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

	return os.WriteFile(path, []byte(out), 0644)
}

// Undo reverts the most recent operation by restoring refs to their before-state.
func (r *Repository) Undo() (*OperationEntry, int, error) {
	entries, err := r.ReadOperations()
	if err != nil {
		return nil, 0, err
	}

	if len(entries) == 0 {
		return nil, 0, fmt.Errorf("no operations to undo")
	}

	last := entries[0]

	var restored int
	for _, change := range last.RefChanges {
		if change.Before == "" {
			_ = r.Store.DeleteRef(change.Ref)
			restored++
		} else {
			if err := r.Store.SaveRef(change.Ref, change.Before); err != nil {
				return nil, 0, fmt.Errorf("failed to restore ref %s: %w", change.Ref, err)
			}
			restored++
		}
	}

	if err := r.RemoveLastOperation(); err != nil {
		// Non-fatal: the refs were already restored.
	}

	return &last, restored, nil
}
