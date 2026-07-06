package memory

import (
	"context"

	"github.com/your-org/drift/internal/core"
)

// GetIndex retrieves the staging index.
func (ms *MemoryStorage) GetIndex(ctx context.Context) (*core.Index, error) {
	if ms.index == nil {
		return &core.Index{}, nil
	}
	return cloneIndex(ms.index), nil
}

// SetIndex stores the staging index.
func (ms *MemoryStorage) SetIndex(ctx context.Context, index *core.Index) error {
	ms.index = cloneIndex(index)
	return nil
}
