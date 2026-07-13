package memory

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
)

// GetIndex retrieves the staging index.
func (ms *MemoryStorage) GetIndex(ctx context.Context) (*core.Index, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.index == nil {
		return &core.Index{}, nil
	}
	return cloneIndex(ms.index), nil
}

// SetIndex stores the staging index.
func (ms *MemoryStorage) SetIndex(ctx context.Context, index *core.Index) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.index = cloneIndex(index)
	return nil
}
