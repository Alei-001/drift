package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// maxTreeEntryNameLen mirrors go-git's limit (upstream Git's
// fsck.treeEntryLargeName.maxTreeEntryLen, 4096 bytes). Entries longer
// than this almost always indicate a malformed or hand-crafted tree.
const maxTreeEntryNameLen = 4096

// Tree validation errors, mirroring go-git's object package.
var (
	ErrEntriesNotSorted = errors.New("entries in tree are not sorted")
	ErrMalformedTree    = errors.New("malformed tree")
	ErrDuplicateEntry   = errors.New("duplicate entry in tree")
	ErrInvalidTree      = errors.New("invalid tree")
)

type TreeEntry struct {
	Name string
	Type ObjectType
	Hash string
	Mode uint32
}

type Tree struct {
	Hash    string
	Entries []TreeEntry
}

func NewTree(entries []TreeEntry) (*Tree, error) {
	sort.Slice(entries, func(i, j int) bool {
		return treeEntrySortName(&entries[i]) < treeEntrySortName(&entries[j])
	})

	t := &Tree{Entries: entries}

	// G1: validate before marshaling, mirroring go-git's Tree.Encode which
	// calls Validate() to prevent writing malformed trees.
	if err := t.Validate(); err != nil {
		return nil, err
	}

	data, err := t.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tree: %w", err)
	}
	t.Hash = CalculateHash(data)
	return t, nil
}

// Validate reports whether the tree object obeys the same structural rules
// upstream Git's fsck_tree enforces. Mirrors go-git's Tree.Validate.
//
// Checks: empty names, slash in names, duplicate entries, name length
// exceeding maxTreeEntryNameLen, bad modes, unsorted entries, null hashes,
// and path traversal / metadata-disguise names (via ValidateTreePath).
func (t *Tree) Validate() error {
	var errs []error
	add := func(err error) {
		errs = append(errs, fmt.Errorf("%w: %w", ErrInvalidTree, err))
	}

	seen := make(map[string]struct{}, len(t.Entries))
	var prevSortName string

	for i := range t.Entries {
		e := &t.Entries[i]

		// Null hash check.
		if e.Hash == "" || e.Hash == nullHash {
			add(fmt.Errorf("entry %q points to null hash", e.Name))
		}

		switch {
		case e.Name == "":
			add(errors.New("contains empty entry name"))
		case strings.ContainsRune(e.Name, '/'):
			add(fmt.Errorf("entry name %q contains a slash", e.Name))
		default:
			// Path traversal / metadata disguise check.
			if err := ValidateTreePath(e.Name); err != nil {
				add(err)
			}
			if _, dup := seen[e.Name]; dup {
				add(fmt.Errorf("%w: %q", ErrDuplicateEntry, e.Name))
			}
			seen[e.Name] = struct{}{}

			if len(e.Name) > maxTreeEntryNameLen {
				add(fmt.Errorf("entry name length %d exceeds %d", len(e.Name), maxTreeEntryNameLen))
			}
		}

		// Mode validation against the canonical set.
		if !isValidTreeMode(e.Mode) {
			add(fmt.Errorf("entry %q has bad mode %o", e.Name, e.Mode))
		}

		// Type/mode consistency.
		if e.Type == TreeObject && e.Mode != ModeDir {
			add(fmt.Errorf("entry %q is TreeObject but mode is not Dir", e.Name))
		}
		if e.Type == BlobObject && (e.Mode == ModeDir) {
			add(fmt.Errorf("entry %q is BlobObject but mode is Dir", e.Name))
		}

		sortName := treeEntrySortName(e)
		if i > 0 && prevSortName > sortName {
			add(ErrEntriesNotSorted)
		}
		prevSortName = sortName
	}

	return errors.Join(errs...)
}

// treeEntrySortName returns the sort key for a tree entry. Git compares
// tree entries as if directory names had a trailing slash. Mirrors
// go-git's treeEntrySortName.
func treeEntrySortName(e *TreeEntry) string {
	if e.Mode == ModeDir || e.Type == TreeObject {
		return e.Name + "/"
	}
	return e.Name
}

// canonicalTreeMode normalizes a mode to one of the canonical Drift modes.
// Mirrors go-git's canonicalTreeMode. Non-canonical bits are stripped so
// that equivalent modes (e.g. 0o100640 → 0o100644) hash identically.
func canonicalTreeMode(mode uint32) uint32 {
	switch mode & 0o170000 {
	case 0o040000:
		return ModeDir
	case 0o100000:
		if mode&0o111 != 0 {
			return ModeExecutable
		}
		return ModeRegular
	case 0o120000:
		return ModeSymlink
	default:
		return mode
	}
}

// isValidTreeMode reports whether mode is one of the canonical Drift modes.
func isValidTreeMode(mode uint32) bool {
	switch mode {
	case ModeRegular, ModeExecutable, ModeSymlink, ModeDir:
		return true
	}
	return false
}
