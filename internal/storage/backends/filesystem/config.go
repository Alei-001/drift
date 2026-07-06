package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
)

// GetConfig reads the drift configuration from the config file.
// The file is unmarshaled on top of DefaultConfig(), so fields absent from
// the JSON retain their default values rather than Go zero values. This
// matters for bool fields like Compression (zero=false vs default=true)
// and for int fields where 0 is not a valid size.
func (fs *FSStorage) GetConfig(ctx context.Context) (*core.Config, error) {
	path := filepath.Join(fs.root, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return core.DefaultConfig(), nil
		}
		return nil, err
	}
	cfg := core.DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", storage.ErrCorrupted)
	}
	// Apply shared normalization (handles negative/empty/zero fields by
	// restoring defaults). Filesystem-specific upper-bound clamps are
	// applied afterwards to prevent OOM on a malicious or mistaken config.
	cfg.Core.Normalize()
	if cfg.Core.ChunkMinSize > storage.MaxChunkMinSize {
		cfg.Core.ChunkMinSize = storage.MaxChunkMinSize
	}
	if cfg.Core.ChunkAvgSize > storage.MaxChunkAvgSize {
		cfg.Core.ChunkAvgSize = storage.MaxChunkAvgSize
	}
	if cfg.Core.ChunkMaxSize > storage.MaxChunkMaxSize {
		cfg.Core.ChunkMaxSize = storage.MaxChunkMaxSize
	}
	return cfg, nil
}

// SetConfig writes the drift configuration to the config file.
func (fs *FSStorage) SetConfig(ctx context.Context, config *core.Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(fs.root, ConfigFile)
	return fsutil.WriteFileAtomic(path, data, 0644)
}
