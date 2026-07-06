package memory

import (
	"context"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

// GetConfig returns the stored configuration, cloned and normalized to
// match the filesystem backend's invariants (chunk size clamping and
// field normalization).
func (ms *MemoryStorage) GetConfig(ctx context.Context) (*core.Config, error) {
	if ms.config == nil {
		return core.DefaultConfig(), nil
	}
	// Clone before returning so callers cannot mutate stored state, and
	// apply shared normalization so tests that SetConfig a partial config
	// observe the same field invariants as the filesystem backend.
	clone := cloneConfig(ms.config)
	clone.Core.Normalize()
	// Apply upper-bound clamps to match the filesystem backend.
	if clone.Core.ChunkMinSize > storage.MaxChunkMinSize {
		clone.Core.ChunkMinSize = storage.MaxChunkMinSize
	}
	if clone.Core.ChunkAvgSize > storage.MaxChunkAvgSize {
		clone.Core.ChunkAvgSize = storage.MaxChunkAvgSize
	}
	if clone.Core.ChunkMaxSize > storage.MaxChunkMaxSize {
		clone.Core.ChunkMaxSize = storage.MaxChunkMaxSize
	}
	return clone, nil
}

// SetConfig stores the configuration.
func (ms *MemoryStorage) SetConfig(ctx context.Context, config *core.Config) error {
	ms.config = cloneConfig(config)
	return nil
}
