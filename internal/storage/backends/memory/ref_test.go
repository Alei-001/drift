package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

func TestDeleteRef_HEAD(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead})

	err := store.DeleteRef(ctx, "HEAD")
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for deleting HEAD, got %v", err)
	}
}

func TestDeleteRef_InvalidName(t *testing.T) {
	store := NewMemoryStorage()
	tests := []string{"", "foo..bar", "foo\\bar"}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			err := store.DeleteRef(context.Background(), name)
			if !errors.Is(err, storage.ErrInvalidRef) {
				t.Errorf("DeleteRef(%q): expected ErrInvalidRef, got %v", name, err)
			}
		})
	}
}

func TestSetRef_NormalizesSymRefPrefix(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	// SetRef should strip the "refs/" prefix from SymRef values.
	store.SetRef(ctx, "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "refs/heads/main",
	})
	store.SetRef(ctx, "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{0x01},
	})

	got, err := store.GetRef(ctx, "HEAD")
	if err != nil {
		t.Fatalf("GetRef HEAD failed: %v", err)
	}
	if got.SymRef != "heads/main" {
		t.Errorf("SymRef should be normalized: got %q, want %q", got.SymRef, "heads/main")
	}
	if got.Target != (core.Hash{0x01}) {
		t.Errorf("Target should resolve through symref: got %v, want %v", got.Target, core.Hash{0x01})
	}
}

func TestSetRef_InvalidSymRef(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	err := store.SetRef(ctx, "HEAD", &core.Reference{
		Name:   "HEAD",
		SymRef: "foo..bar", // invalid ref name
	})
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for invalid symref, got %v", err)
	}
}

func TestGetRef_SymRefDepthLimit(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()

	// Build a chain of symrefs longer than MaxSymRefDepth.
	// ref0 -> ref1 -> ... -> refN -> heads/main
	store.SetRef(ctx, "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{0xaa},
	})
	prev := "heads/main"
	for i := 0; i < storage.MaxSymRefDepth+2; i++ {
		name := "sym" + string(rune('a'+i))
		store.SetRef(ctx, name, &core.Reference{
			Name:   name,
			SymRef: prev,
		})
		prev = name
	}

	// Resolving the deepest symref should hit the recursion limit.
	_, err := store.GetRef(ctx, prev)
	if err == nil {
		t.Fatal("expected error for exceeding symref depth, got nil")
	}
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for symref recursion limit, got %v", err)
	}
}

func TestGetRef_DanglingSymRef(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/nonexistent",
	})

	_, err := store.GetRef(ctx, "HEAD")
	if err == nil {
		t.Fatal("expected error for dangling symref, got nil")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound for dangling symref, got %v", err)
	}
}

func TestListRefs_ExcludesHEAD(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch})
	store.SetRef(ctx, "tags/v1", &core.Reference{Name: "tags/v1", Type: core.RefTypeTag})
	store.SetRef(ctx, "HEAD", &core.Reference{Name: "HEAD", Type: core.RefTypeHead, SymRef: "heads/main"})

	refs, err := store.ListRefs(ctx, "")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	for _, r := range refs {
		if r.Name == "HEAD" {
			t.Error("ListRefs should exclude HEAD")
		}
	}
	if len(refs) != 2 {
		t.Errorf("expected 2 refs (excluding HEAD), got %d", len(refs))
	}
}

func TestListRefs_DanglingSymRefSkipped(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: core.Hash{0x01}})
	store.SetRef(ctx, "heads/dangling", &core.Reference{Name: "heads/dangling", SymRef: "heads/nonexistent"})

	refs, err := store.ListRefs(ctx, "heads/")
	if err != nil {
		t.Fatalf("ListRefs failed: %v", err)
	}
	// heads/dangling should be skipped (dangling symref → ErrNotFound).
	for _, r := range refs {
		if r.Name == "heads/dangling" {
			t.Error("dangling symref should be skipped in ListRefs")
		}
	}
	if len(refs) != 1 {
		t.Errorf("expected 1 valid ref, got %d", len(refs))
	}
}

func TestGetRef_DerivesTypeFromName(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "wrong", Type: core.RefTypeTag, Target: core.Hash{0x01}})
	store.SetRef(ctx, "tags/v1", &core.Reference{Name: "wrong", Type: core.RefTypeBranch, Target: core.Hash{0x02}})
	store.SetRef(ctx, "HEAD", &core.Reference{Name: "wrong", Type: core.RefTypeTag, SymRef: "heads/main"})

	got, _ := store.GetRef(ctx, "heads/main")
	if got.Type != core.RefTypeBranch {
		t.Errorf("heads/main Type: got %q, want %q", got.Type, core.RefTypeBranch)
	}
	got, _ = store.GetRef(ctx, "tags/v1")
	if got.Type != core.RefTypeTag {
		t.Errorf("tags/v1 Type: got %q, want %q", got.Type, core.RefTypeTag)
	}
	got, _ = store.GetRef(ctx, "HEAD")
	if got.Type != core.RefTypeHead {
		t.Errorf("HEAD Type: got %q, want %q", got.Type, core.RefTypeHead)
	}
}

func TestGetRef_ReturnedNameMatchesQuery(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "stored-name", Target: core.Hash{0x01}})

	got, _ := store.GetRef(ctx, "heads/main")
	if got.Name != "heads/main" {
		t.Errorf("Name: got %q, want %q (should match query, not stored)", got.Name, "heads/main")
	}
}

func TestSetRef_Overwrite(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "heads/main", Target: core.Hash{0x01}})
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "heads/main", Target: core.Hash{0x02}})

	got, _ := store.GetRef(ctx, "heads/main")
	if got.Target != (core.Hash{0x02}) {
		t.Errorf("Target after overwrite: got %v, want %v", got.Target, core.Hash{0x02})
	}
}

func TestListRefs_PrefixFilter(t *testing.T) {
	store := NewMemoryStorage()
	ctx := context.Background()
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: core.Hash{0x01}})
	store.SetRef(ctx, "heads/dev", &core.Reference{Name: "heads/dev", Type: core.RefTypeBranch, Target: core.Hash{0x02}})
	store.SetRef(ctx, "tags/v1", &core.Reference{Name: "tags/v1", Type: core.RefTypeTag, Target: core.Hash{0x03}})
	store.SetRef(ctx, "tags/v2", &core.Reference{Name: "tags/v2", Type: core.RefTypeTag, Target: core.Hash{0x04}})

	headRefs, _ := store.ListRefs(ctx, "heads/")
	if len(headRefs) != 2 {
		t.Errorf("heads/ prefix: expected 2 refs, got %d", len(headRefs))
	}
	for _, r := range headRefs {
		if !strings.HasPrefix(r.Name, "heads/") {
			t.Errorf("ref %q does not match heads/ prefix", r.Name)
		}
	}

	tagRefs, _ := store.ListRefs(ctx, "tags/")
	if len(tagRefs) != 2 {
		t.Errorf("tags/ prefix: expected 2 refs, got %d", len(tagRefs))
	}
}
