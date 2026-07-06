package filesystem

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/refname"
	"github.com/your-org/drift/internal/util/fsutil"
)

// Compile-time assertion that FSStorage implements storage.Storer.
var _ storage.Storer = (*FSStorage)(nil)

func (fs *FSStorage) GetRef(ctx context.Context, name string) (*core.Reference, error) {
	return fs.getRefRecursive(ctx, name, 0)
}

func (fs *FSStorage) getRefRecursive(ctx context.Context, name string, depth int) (*core.Reference, error) {
	if depth > storage.MaxSymRefDepth {
		return nil, fmt.Errorf("symref recursion limit exceeded at %q: %w", name, storage.ErrInvalidRef)
	}
	if err := refname.Validate(name); err != nil {
		return nil, err
	}

	var path string
	if name == "HEAD" {
		path = filepath.Join(fs.root, HeadFile)
	} else {
		path = filepath.Join(fs.root, RefsDir, name)
	}

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
		symRef = strings.TrimPrefix(symRef, RefsDir+"/")
		target, err := fs.getRefRecursive(ctx, symRef, depth+1)
		if err != nil {
			return nil, err
		}
		return &core.Reference{
			Name:   name,
			Type:   refname.RefType(name),
			SymRef: symRef,
			Target: target.Target,
		}, nil
	}

	b, err := hex.DecodeString(content)
	if err != nil {
		return nil, fmt.Errorf("decode ref %q target: %w", name, storage.ErrInvalidRef)
	}
	if len(b) != core.HashSize {
		return nil, fmt.Errorf("ref %q target length %d, expected %d: %w", name, len(b), core.HashSize, storage.ErrInvalidRef)
	}
	var h core.Hash
	copy(h[:], b)
	ref := &core.Reference{
		Name:   name,
		Target: h,
		Type:   refname.RefType(name),
	}
	return ref, nil
}

func (fs *FSStorage) SetRef(ctx context.Context, name string, ref *core.Reference) error {
	if err := refname.Validate(name); err != nil {
		return err
	}

	var path string
	if name == "HEAD" {
		path = filepath.Join(fs.root, HeadFile)
	} else {
		path = filepath.Join(fs.root, RefsDir, name)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, fsutil.DefaultDirPerm); err != nil {
			return err
		}
	}

	if ref.SymRef != "" {
		symTarget := ref.SymRef
		if strings.HasPrefix(symTarget, RefsDir+"/") {
			symTarget = strings.TrimPrefix(symTarget, RefsDir+"/")
		}
		if err := refname.Validate(symTarget); err != nil {
			return err
		}
		return fsutil.WriteFileAtomic(path, []byte("ref: "+RefsDir+"/"+symTarget+"\n"), fsutil.DefaultFilePerm)
	}
	hexStr := ref.Target.FullString()
	return fsutil.WriteFileAtomic(path, []byte(hexStr+"\n"), fsutil.DefaultFilePerm)
}

func (fs *FSStorage) ListRefs(ctx context.Context, prefix string) ([]*core.Reference, error) {
	// Bail out early if the caller has already cancelled, before we start
	// walking the refs directory.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	refsRoot := filepath.Join(fs.root, RefsDir)
	var refs []*core.Reference
	err := filepath.WalkDir(refsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
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
			// Skip not-found refs (e.g. .DS_Store, corrupt refs) but
			// propagate other errors instead of aborting silently.
			if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrInvalidRef) {
				return nil
			}
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

func (fs *FSStorage) DeleteRef(ctx context.Context, name string) error {
	if err := refname.Validate(name); err != nil {
		return err
	}
	if name == "HEAD" {
		return fmt.Errorf("cannot delete HEAD: %w", storage.ErrInvalidRef)
	}
	path := filepath.Join(fs.root, RefsDir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete ref %q: %w", name, err)
	}
	return nil
}
