package filesystem

import (
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
func (fs *FSStorage) GetRef(name string) (*core.Reference, error) {
	if err := validateRefName(name); err != nil {
		return nil, err
	}
	path := filepath.Join(fs.root, RefsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	hexStr := strings.TrimSpace(string(data))
	b, err := hex.DecodeString(hexStr)
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
func (fs *FSStorage) SetRef(name string, ref *core.Reference) error {
	if err := validateRefName(name); err != nil {
		return err
	}
	path := filepath.Join(fs.root, RefsDir, name)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	hexStr := ref.Target.FullString()
	return fsutil.WriteFileAtomic(path, []byte(hexStr+"\n"), 0644)
}

// ListRefs lists all references matching the given prefix.
func (fs *FSStorage) ListRefs(prefix string) ([]*core.Reference, error) {
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
		if !strings.HasPrefix(rel, prefix) {
			return nil
		}
		ref, err := fs.GetRef(rel)
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
func (fs *FSStorage) DeleteRef(name string) error {
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
