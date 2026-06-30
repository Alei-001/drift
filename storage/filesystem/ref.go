package filesystem

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/util/fsutil"
)

// validateRefName checks that a ref name does not contain path traversal characters.
func validateRefName(name string) error {
	if name == "" {
		return fmt.Errorf("ref name is empty")
	}
	// Check for path traversal
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid ref name: %q contains '..'", name)
	}
	// Ensure the cleaned path stays within the refs directory
	cleaned := filepath.Clean(name)
	if strings.HasPrefix(cleaned, "..") || cleaned == ".." {
		return fmt.Errorf("invalid ref name: %q escapes refs directory", name)
	}
	return nil
}

// GetRef reads a reference from the refs directory.
// For HEAD, if the file contains a symbolic reference ("ref: heads/main"),
// the SymRef field is populated and Target is resolved by recursively
// reading the referenced branch.
func (fs *FSStorage) GetRef(ctx context.Context, name string) (*core.Reference, error) {
	if err := validateRefName(name); err != nil {
		return nil, err
	}
	path := filepath.Join(fs.root, RefsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(string(data))

	if name == "HEAD" && strings.HasPrefix(content, "ref: ") {
		symRef := strings.TrimSpace(content[len("ref: "):])
		target, err := fs.GetRef(ctx, symRef)
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
		return nil, err
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
