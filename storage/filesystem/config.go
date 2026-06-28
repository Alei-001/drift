package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/util/fsutil"
)

// GetConfig reads the drift configuration from the config file.
func (fs *FSStorage) GetConfig() (*core.Config, error) {
	path := filepath.Join(fs.root, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return core.DefaultConfig(), nil
		}
		return nil, err
	}
	cfg := &core.Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Core.ChunkMinSize <= 0 {
		cfg.Core.ChunkMinSize = 131072
	}
	if cfg.Core.ChunkAvgSize <= 0 {
		cfg.Core.ChunkAvgSize = 262144
	}
	if cfg.Core.ChunkMaxSize <= 0 {
		cfg.Core.ChunkMaxSize = 524288
	}
	if cfg.Core.IgnoreFile == "" {
		cfg.Core.IgnoreFile = ".driftignore"
	}
	return cfg, nil
}

// SetConfig writes the drift configuration to the config file.
func (fs *FSStorage) SetConfig(config *core.Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(fs.root, ConfigFile)
	return fsutil.WriteFileAtomic(path, data, 0644)
}
