package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drift/drift/internal/storage"
	"github.com/spf13/cobra"
)

// Operation log records user actions that modify repository state, enabling
// an undo safety net. This is a friendly version of Git's reflog — instead
// of exposing HEAD@{1} syntax, users see a readable history and can undo
// the most recent operation.
//
// Storage: .drift/operations.log (append-only JSON lines)
//
// Each entry records:
//   - Timestamp
//   - Operation type (save, switch, branch-delete, branch-rename, restore)
//   - Description (human-readable)
//   - Before/after state of affected refs

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
	Timestamp time.Time      `json:"timestamp"`
	Op        OpType         `json:"op"`
	Desc      string         `json:"desc"`
	RefChanges []RefChange   `json:"ref_changes"`
}

// RefChange records the before/after value of a single ref.
type RefChange struct {
	Ref    string `json:"ref"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// recordOperation appends an operation entry to the operations log.
func recordOperation(store *storage.Store, op OpType, desc string, changes []RefChange) {
	entry := OperationEntry{
		Timestamp:  time.Now(),
		Op:         op,
		Desc:       desc,
		RefChanges: changes,
	}

	path := filepath.Join(store.DriftDir(), operationsFile)
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	// Append to the log file (create if doesn't exist).
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}

// readOperations reads all operations from the log, newest first.
func readOperations(store *storage.Store) ([]OperationEntry, error) {
	path := filepath.Join(store.DriftDir(), operationsFile)
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

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show recent operations (for undo reference)",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := readOperations(sharedStore)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No operations recorded yet")
			return nil
		}

		fmt.Printf("Recent operations (newest first):\n\n")
		limit := 20
		if len(entries) < limit {
			limit = len(entries)
		}
		for i := 0; i < limit; i++ {
			e := entries[i]
			fmt.Printf("  %d. %s  %s  %s\n", i+1, e.Timestamp.Format("2006-01-02 15:04:05"), e.Op, e.Desc)
		}

		if len(entries) > 20 {
			fmt.Printf("\n(%d more older operations — not shown)\n", len(entries)-20)
		}

		fmt.Println("\nTo undo the most recent operation: drift undo")
		return nil
	},
}

var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Undo the most recent operation",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := readOperations(sharedStore)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			return fmt.Errorf("no operations to undo")
		}

		last := entries[0]

		// Restore refs to their before-state.
		var restored int
		for _, change := range last.RefChanges {
			if change.Before == "" {
				// Ref didn't exist before — delete it.
				_ = sharedStore.DeleteRef(change.Ref)
				restored++
			} else {
				if err := sharedStore.SaveRef(change.Ref, change.Before); err != nil {
					return fmt.Errorf("failed to restore ref %s: %w", change.Ref, err)
				}
				restored++
			}
		}

		// Remove the undone entry from the log so a second undo goes further back.
		if err := removeLastOperation(sharedStore); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not update operations log: %v\n", err)
		}

		fmt.Printf("Undid: %s (%s)\n", last.Desc, last.Op)
		fmt.Printf("Restored %d ref(s) to previous state.\n", restored)
		return nil
	},
}

// removeLastOperation removes the last (most recent) entry from the operations log.
func removeLastOperation(store *storage.Store) error {
	path := filepath.Join(store.DriftDir(), operationsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	// Remove trailing empty line if present.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	// Remove the last line.
	lines = lines[:len(lines)-1]
	out := strings.Join(lines, "\n")
	if len(lines) > 0 {
		out += "\n"
	}

	return os.WriteFile(path, []byte(out), 0644)
}

func init() {
	rootCmd.AddCommand(historyCmd)
	rootCmd.AddCommand(undoCmd)
}
