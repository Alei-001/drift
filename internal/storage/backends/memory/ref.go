package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/storage/refname"
)

// GetRef reads a reference. If the reference is a symbolic reference,
// Target is resolved by recursively reading the referenced ref.
func (ms *MemoryStorage) GetRef(ctx context.Context, name string) (*core.Reference, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.getRefRecursiveLocked(ctx, name, 0)
}

// getRefRecursiveLocked resolves a reference, following symbolic refs.
// The caller MUST hold ms.mu (read or write lock).
func (ms *MemoryStorage) getRefRecursiveLocked(ctx context.Context, name string, depth int) (*core.Reference, error) {
	if depth > storage.MaxSymRefDepth {
		return nil, fmt.Errorf("symref recursion limit exceeded at %q: %w", name, storage.ErrInvalidRef)
	}
	if err := refname.Validate(name); err != nil {
		return nil, fmt.Errorf("validate ref %q: %w", name, err)
	}
	ref, ok := ms.refs[name]
	if !ok {
		return nil, fmt.Errorf("get ref %q: %w", name, storage.ErrNotFound)
	}
	if ref.SymRef != "" {
		target, err := ms.getRefRecursiveLocked(ctx, ref.SymRef, depth+1)
		if err != nil {
			return nil, fmt.Errorf("resolve symref %q: %w", ref.SymRef, err)
		}
		resolved := cloneReference(ref)
		resolved.Name = name
		resolved.Target = target.Target
		// Derive Type from name, matching the filesystem backend's behavior
		// (the stored Type field is ignored so both backends agree).
		resolved.Type = refname.RefType(name)
		return resolved, nil
	}
	clone := cloneReference(ref)
	clone.Name = name
	clone.Type = refname.RefType(name)
	return clone, nil
}

// SetRef writes a reference.
func (ms *MemoryStorage) SetRef(ctx context.Context, name string, ref *core.Reference) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if err := refname.Validate(name); err != nil {
		return fmt.Errorf("validate ref %q: %w", name, err)
	}
	clone := cloneReference(ref)
	if clone.SymRef != "" {
		// Normalize SymRef by stripping any "refs/" prefix so subsequent
		// GetRef lookups succeed regardless of how the caller wrote it.
		// This mirrors the filesystem backend's on-disk format.
		clone.SymRef = strings.TrimPrefix(clone.SymRef, "refs/")
		if err := refname.Validate(clone.SymRef); err != nil {
			return fmt.Errorf("validate symref %q: %w", clone.SymRef, err)
		}
	}
	ms.refs[name] = clone
	return nil
}

// ListRefs lists all references matching the given prefix.
// HEAD is excluded to match the filesystem backend, which only walks the
// refs/ directory (HEAD lives at the repository root, outside refs/).
// Each ref is resolved via getRefRecursiveLocked so symrefs have their
// Target populated and Type derived from the name, matching the
// filesystem backend.
//
// Only ErrNotFound errors from resolution are skipped (e.g. dangling
// symref); other errors are propagated so callers can distinguish I/O
// failures from missing refs.
func (ms *MemoryStorage) ListRefs(ctx context.Context, prefix string) ([]*core.Reference, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	var refs []*core.Reference
	for name := range ms.refs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if name == "HEAD" {
			continue
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		ref, err := ms.getRefRecursiveLocked(ctx, name, 0)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("list refs: resolve %q: %w", name, err)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// DeleteRef removes a reference.
func (ms *MemoryStorage) DeleteRef(ctx context.Context, name string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if err := refname.Validate(name); err != nil {
		return fmt.Errorf("validate ref %q: %w", name, err)
	}
	if name == "HEAD" {
		return fmt.Errorf("cannot delete HEAD: %w", storage.ErrInvalidRef)
	}
	delete(ms.refs, name)
	return nil
}
