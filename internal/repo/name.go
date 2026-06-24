package repo

import (
	"fmt"
	"strings"
)

// ValidateNameLabel checks that a name label is safe to use as a filename.
func ValidateNameLabel(label string) error {
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
	return nil
}

// AddName assigns a human-readable label to a version.
func (r *Repository) AddName(version, label string) error {
	if err := ValidateNameLabel(label); err != nil {
		return err
	}

	commit, err := r.ResolveCommit(version)
	if err != nil {
		return err
	}

	refName := "names/" + label
	if err := r.Store.SaveRef(refName, commit.Hash); err != nil {
		return fmt.Errorf("failed to save name: %w", err)
	}
	return nil
}

// DeleteName removes a version name.
func (r *Repository) DeleteName(label string) error {
	refName := "names/" + label
	if _, err := r.Store.GetRef(refName); err != nil {
		return fmt.Errorf("name '%s' not found", label)
	}
	if err := r.Store.DeleteRef(refName); err != nil {
		return fmt.Errorf("failed to delete name: %w", err)
	}
	return nil
}

// ResolveName checks if a version string is a name alias and returns the
// commit hash if so. Returns empty string if not a name.
func (r *Repository) ResolveName(version string) string {
	refName := "names/" + version
	hash, err := r.Store.GetRef(refName)
	if err != nil || hash == "" {
		return ""
	}
	return hash
}

// NameEntry represents a version name alias.
type NameEntry struct {
	Label string
	Hash  string
}

// ListNames returns all version name aliases.
func (r *Repository) ListNames() ([]NameEntry, error) {
	refs, err := r.Store.ListRefs()
	if err != nil {
		return nil, err
	}

	var entries []NameEntry
	for refName, hash := range refs {
		if strings.HasPrefix(refName, "names/") {
			label := strings.TrimPrefix(refName, "names/")
			entries = append(entries, NameEntry{Label: label, Hash: hash})
		}
	}
	return entries, nil
}
