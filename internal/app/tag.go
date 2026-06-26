package app

import (
	"fmt"
	"sort"
	"strings"
)

type TagEntry struct {
	Label   string
	Hash    string
	ID      string
	Message string
}

func validateTagLabel(label string) error {
	if label == "" {
		return fmt.Errorf("tag cannot be empty")
	}
	if len(label) > 100 {
		return fmt.Errorf("tag too long (max 100 characters)")
	}
	if strings.ContainsAny(label, `/\`) {
		return fmt.Errorf("tag cannot contain path separators")
	}
	if label == "." || label == ".." {
		return fmt.Errorf("tag cannot be '.' or '..'")
	}
	for _, c := range label {
		if c == 0 || c < 0x20 {
			return fmt.Errorf("tag cannot contain null bytes or control characters")
		}
	}
	for _, c := range label {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return fmt.Errorf("tag can only contain letters, digits, '.', '_', and '-'")
		}
	}
	return nil
}

func (a *App) TagAdd(version, label string) error {
	if err := validateTagLabel(label); err != nil {
		return err
	}

	commit, err := a.ResolveCommit(version)
	if err != nil {
		return err
	}

	refName := "tags/" + label

	// Refuse to overwrite an existing tag (mirrors git tag behavior).
	if existing, err := a.store.GetRef(refName); err == nil && existing != "" {
		return fmt.Errorf("tag %q already exists (delete it first with 'drift tag --delete %s')", label, label)
	}

	if err := a.store.SaveRef(refName, commit.Hash); err != nil {
		return fmt.Errorf("failed to save tag: %w", err)
	}

	if err := a.recordOperation(OpTagAdd, fmt.Sprintf("tag %s as %s", commit.ID, label), []RefChange{
		{Ref: refName, Before: "", After: commit.Hash},
	}); err != nil {
		return err
	}
	return nil
}

func (a *App) TagDelete(label string) error {
	refName := "tags/" + label
	oldHash, err := a.store.GetRef(refName)
	if err != nil {
		return fmt.Errorf("tag '%s' not found", label)
	}
	if err := a.store.DeleteRef(refName); err != nil {
		return fmt.Errorf("failed to delete tag: %w", err)
	}

	if err := a.recordOperation(OpTagDelete, fmt.Sprintf("delete tag %s", label), []RefChange{
		{Ref: refName, Before: oldHash, After: ""},
	}); err != nil {
		return err
	}
	return nil
}

func (a *App) TagList() ([]TagEntry, error) {
	refs, err := a.store.ListRefs()
	if err != nil {
		return nil, err
	}

	var entries []TagEntry
	for refName, hash := range refs {
		if strings.HasPrefix(refName, "tags/") {
			label := strings.TrimPrefix(refName, "tags/")
			entry := TagEntry{Label: label, Hash: hash}
			// Best-effort: populate ID and Message from commit
			if commit, err := a.findCommitByHash(hash); err == nil {
				entry.ID = commit.ID
				entry.Message = commit.Message
			}
			entries = append(entries, entry)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Label < entries[j].Label
	})

	return entries, nil
}

func (a *App) TagsByHash() map[string][]string {
	entries, err := a.TagList()
	if err != nil {
		return nil
	}
	result := make(map[string][]string)
	for _, e := range entries {
		result[e.Hash] = append(result[e.Hash], e.Label)
	}
	return result
}
