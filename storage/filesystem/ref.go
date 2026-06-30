package filesystem

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
)

// validateRefName checks that a ref name is safe. It rejects empty names,
// path traversal, control characters, spaces, names starting with '-',
// and Windows reserved device names.
func validateRefName(name string) error {
	if name == "" {
		return fmt.Errorf("ref name is empty: %w", storage.ErrInvalidRef)
	}
	// Reject control characters and spaces
	for _, c := range name {
		if c < 0x20 || c == 0x7F {
			return fmt.Errorf("invalid ref name: %q contains control character: %w", name, storage.ErrInvalidRef)
		}
		if c == ' ' {
			return fmt.Errorf("invalid ref name: %q contains space: %w", name, storage.ErrInvalidRef)
		}
	}
	// Reject names starting with '-' (CLI flag confusion)
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid ref name: %q starts with '-': %w", name, storage.ErrInvalidRef)
	}
	// Check for path traversal
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid ref name: %q contains '..': %w", name, storage.ErrInvalidRef)
	}
	// Ensure the cleaned path stays within the refs directory
	cleaned := filepath.Clean(name)
	if strings.HasPrefix(cleaned, "..") || cleaned == ".." {
		return fmt.Errorf("invalid ref name: %q escapes refs directory: %w", name, storage.ErrInvalidRef)
	}
	// Reject Windows reserved device names (case-insensitive)
	base := strings.ToLower(filepath.Base(name))
	if isWindowsReservedName(base) {
		return fmt.Errorf("invalid ref name: %q is a reserved name: %w", name, storage.ErrInvalidRef)
	}
	return nil
}

// isWindowsReservedName checks if the name is a Windows reserved device
// name (CON, AUX, NUL, COM1-9, LPT1-9), case-insensitive.
func isWindowsReservedName(name string) bool {
	switch name {
	case "con", "aux", "nul", "prn":
		return true
	}
	if len(name) >= 4 {
		switch name[:3] {
		case "com", "lpt":
			if name[3] >= '1' && name[3] <= '9' {
				return true
			}
		}
	}
	return false
}

// maxSymRefDepth bounds the number of symbolic-reference hops GetRef will
// follow before giving up. It guards against malformed or malicious
// self-referential symrefs (e.g. HEAD -> HEAD) that would otherwise cause
// unbounded recursion.
const maxSymRefDepth = 8

// GetRef reads a reference from the refs directory. If the file contains a
// symbolic reference ("ref: heads/main"), the SymRef field is populated and
// Target is resolved by recursively reading the referenced ref. Symbolic
// references are resolved for all refs, not just HEAD.
func (fs *FSStorage) GetRef(ctx context.Context, name string) (*core.Reference, error) {
	return fs.getRefRecursive(ctx, name, 0)
}

func (fs *FSStorage) getRefRecursive(ctx context.Context, name string, depth int) (*core.Reference, error) {
	if depth > maxSymRefDepth {
		return nil, fmt.Errorf("symref recursion limit exceeded at %q: %w", name, storage.ErrInvalidRef)
	}
	if err := validateRefName(name); err != nil {
		return nil, err
	}
	path := filepath.Join(fs.root, RefsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("get ref %q: %w", name, storage.ErrNotFound)
		}
		return nil, err
	}
	content := strings.TrimSpace(string(data))

	if strings.HasPrefix(content, "ref: ") {
		symRef := strings.TrimSpace(content[len("ref: "):])
		target, err := fs.getRefRecursive(ctx, symRef, depth+1)
		if err != nil {
			return nil, err
		}
		return &core.Reference{
			Name:   name,
			Type:   refType(name),
			SymRef: symRef,
			Target: target.Target,
		}, nil
	}

	b, err := hex.DecodeString(content)
	if err != nil {
		return nil, fmt.Errorf("decode ref %q target: %w", name, storage.ErrInvalidRef)
	}
	var h core.Hash
	copy(h[:], b)
	ref := &core.Reference{
		Name:   name,
		Target: h,
		Type:   refType(name),
	}
	return ref, nil
}

// SetRef writes a reference to the refs directory.
// If ref.SymRef is non-empty, a symbolic reference ("ref: <target>") is
// written instead of a hash.
func (fs *FSStorage) SetRef(ctx context.Context, name string, ref *core.Reference) error {
	if err := validateRefName(name); err != nil {
		return err
	}
	path := filepath.Join(fs.root, RefsDir, name)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if ref.SymRef != "" {
		if err := validateRefName(ref.SymRef); err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(path, []byte("ref: "+ref.SymRef+"\n"), 0644)
	}
	hexStr := ref.Target.FullString()
	return fsutil.WriteFileAtomic(path, []byte(hexStr+"\n"), 0644)
}

// ListRefs lists all references matching the given prefix.
func (fs *FSStorage) ListRefs(ctx context.Context, prefix string) ([]*core.Reference, error) {
	refsRoot := filepath.Join(fs.root, RefsDir)
	var refs []*core.Reference
	err := filepath.WalkDir(refsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(refsRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, prefix) {
			return nil
		}
		ref, err := fs.GetRef(ctx, rel)
		if err != nil {
			return err
		}
		refs = append(refs, ref)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return refs, nil
		}
		return nil, err
	}
	return refs, nil
}

// DeleteRef removes a reference file.
func (fs *FSStorage) DeleteRef(ctx context.Context, name string) error {
	if err := validateRefName(name); err != nil {
		return err
	}
	path := filepath.Join(fs.root, RefsDir, name)
	return os.Remove(path)
}

func refType(name string) core.RefType {
	if name == "HEAD" {
		return core.RefTypeHead
	}
	if strings.HasPrefix(name, HeadsDir+"/") {
		return core.RefTypeBranch
	}
	if strings.HasPrefix(name, TagsDir+"/") {
		return core.RefTypeTag
	}
	return core.RefTypeBranch
}
