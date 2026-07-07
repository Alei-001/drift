package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
)

func TestPutPreview_NoOp(t *testing.T) {
	store := NewMemoryStorage()
	// PutPreview is a no-op stub in the memory backend; it should not error.
	if err := store.PutPreview(context.Background(), core.Hash{0x01}, 100, []byte("preview")); err != nil {
		t.Errorf("PutPreview returned error: %v", err)
	}
}

func TestGetPreview_AlwaysNotFound(t *testing.T) {
	store := NewMemoryStorage()
	// Even after PutPreview, GetPreview returns ErrNotFound because the
	// memory backend's preview store is a no-op stub.
	store.PutPreview(context.Background(), core.Hash{0x01}, 100, []byte("preview"))
	_, err := store.GetPreview(context.Background(), core.Hash{0x01}, 100)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPreview_NotFoundForMissingHash(t *testing.T) {
	store := NewMemoryStorage()
	_, err := store.GetPreview(context.Background(), core.Hash{0xff}, 50)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing hash, got %v", err)
	}
}
