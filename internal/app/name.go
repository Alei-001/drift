package app

import (
	"fmt"
	"strings"
)

type NameEntry struct {
	Label   string
	Hash    string
	ID      string
	Message string
}

func validateNameLabel(label string) error {
	if label == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if len(label) > 100 {
		return fmt.Errorf("name too long (max 100 characters)")
	}
	if strings.ContainsAny(label, `/\`) {
		return fmt.Errorf("name cannot contain path separators")
	}
	if label == "." || label == ".." {
		return fmt.Errorf("name cannot be '.' or '..'")
	}
	for _, c := range label {
		if c == 0 || c < 0x20 {
			return fmt.Errorf("name cannot contain null bytes or control characters")
		}
	}
	return nil
}

func (a *App) NameAdd(version, label string) error {
	if err := validateNameLabel(label); err != nil {
		return err
	}

	commit, err := a.ResolveCommit(version)
	if err != nil {
		return err
	}

	refName := "names/" + label
	if err := a.store.SaveRef(refName, commit.Hash); err != nil {
		return fmt.Errorf("failed to save name: %w", err)
	}
	return nil
}

func (a *App) NameDelete(label string) error {
	refName := "names/" + label
	if _, err := a.store.GetRef(refName); err != nil {
		return fmt.Errorf("name '%s' not found", label)
	}
	if err := a.store.DeleteRef(refName); err != nil {
		return fmt.Errorf("failed to delete name: %w", err)
	}
	return nil
}

func (a *App) NameList() ([]NameEntry, error) {
	refs, err := a.store.ListRefs()
	if err != nil {
		return nil, err
	}

	var entries []NameEntry
	for refName, hash := range refs {
		if strings.HasPrefix(refName, "names/") {
			label := strings.TrimPrefix(refName, "names/")
			entry := NameEntry{Label: label, Hash: hash}
			// Best-effort: populate ID and Message from commit
			if commit, err := a.findCommitByHash(hash); err == nil {
				entry.ID = commit.ID
				entry.Message = commit.Message
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func (a *App) NamesByHash() map[string][]string {
	entries, err := a.NameList()
	if err != nil {
		return nil
	}
	result := make(map[string][]string)
	for _, e := range entries {
		result[e.Hash] = append(result[e.Hash], e.Label)
	}
	return result
}
