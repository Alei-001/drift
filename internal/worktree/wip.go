package worktree

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
)

const wipDir = "wip"

// WIPEntry stores a single staged file's metadata for WIP recovery.
type WIPEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Mode uint32 `json:"mode"`
}

// WIPData is the serialized WIP state for a branch.
type WIPData struct {
	Branch  string    `json:"branch"`
	Entries []WIPEntry `json:"entries"`
}

// SaveWIP saves the current index entries to a WIP file for the given branch.
func (w *Worktree) SaveWIP(branch string, idx *core.Index) error {
	wip := WIPData{Branch: branch}
	for _, e := range idx.Entries {
		wip.Entries = append(wip.Entries, WIPEntry{
			Path: e.Path,
			Hash: e.Hash,
			Mode: e.Mode,
		})
	}

	dir := filepath.Join(w.Store.DriftDir(), wipDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(dir, branch+".json")
	data, err := json.MarshalIndent(wip, "", "  ")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadWIP loads the WIP data for a branch. Returns nil if no WIP exists.
func LoadWIP(store *storage.Store, branch string) (*WIPData, error) {
	path := filepath.Join(store.DriftDir(), wipDir, branch+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var wip WIPData
	if err := json.Unmarshal(data, &wip); err != nil {
		return nil, err
	}
	return &wip, nil
}

// DeleteWIP removes the WIP file for a branch.
func DeleteWIP(store *storage.Store, branch string) error {
	path := filepath.Join(store.DriftDir(), wipDir, branch+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListWIPBranches returns the names of branches that have saved WIP.
func ListWIPBranches(store *storage.Store) ([]string, error) {
	dir := filepath.Join(store.DriftDir(), wipDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var branches []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		branch := strings.TrimSuffix(e.Name(), ".json")
		branches = append(branches, branch)
	}
	return branches, nil
}
