package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
)

// Clamp chunk sizes to reasonable ranges to prevent OOM.
const (
	maxChunkMinSize = 16 * 1024 * 1024  // 16MB
	maxChunkAvgSize = 64 * 1024 * 1024  // 64MB
	maxChunkMaxSize = 256 * 1024 * 1024 // 256MB
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
	// Chunk sizes of 0 mean "use engine defaults" — each filetype engine
	// (text, binary, image, video) applies its own appropriate defaults.
	// Do NOT override them here; only guard against negative values.
	if cfg.Core.ChunkMinSize < 0 {
		cfg.Core.ChunkMinSize = 0
	}
	if cfg.Core.ChunkAvgSize < 0 {
		cfg.Core.ChunkAvgSize = 0
	}
	if cfg.Core.ChunkMaxSize < 0 {
		cfg.Core.ChunkMaxSize = 0
	}
	if cfg.Core.ChunkMinSize > maxChunkMinSize {
		cfg.Core.ChunkMinSize = maxChunkMinSize
	}
	if cfg.Core.ChunkAvgSize > maxChunkAvgSize {
		cfg.Core.ChunkAvgSize = maxChunkAvgSize
	}
	if cfg.Core.ChunkMaxSize > maxChunkMaxSize {
		cfg.Core.ChunkMaxSize = maxChunkMaxSize
	}
	if cfg.Core.IgnoreFile == "" {
		cfg.Core.IgnoreFile = ".driftignore"
	}
	if cfg.Core.AutoSaveInterval <= 0 {
		cfg.Core.AutoSaveInterval = 300
	}
	if cfg.Core.AutoSaveKeep <= 0 {
		cfg.Core.AutoSaveKeep = 10
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
