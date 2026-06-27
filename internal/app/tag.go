package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/drift/drift/internal/storage"
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
	if label == "." || label == ".." {
		return fmt.Errorf("tag cannot be '.' or '..'")
	}
	for _, c := range label {
		if c <= 0x1F || c == 0x7F {
			return fmt.Errorf("tag cannot contain control characters")
		}
		switch c {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return fmt.Errorf("tag cannot contain character: %q", c)
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
	existing, err := a.store.GetRef(refName)
	if err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
		return fmt.Errorf("failed to check tag existence: %w", err)
	}
	if existing != "" {
		return fmt.Errorf("tag %q already exists (delete it first with 'drift tag remove %s')", label, label)
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
