package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/Alei-001/drift/internal/util/fsutil"
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
		return nil, fmt.Errorf("read config: %w", mapOSError(err))
	}
	cfg := core.DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", storage.ErrCorrupted)
	}
	// Apply shared normalization (negative/empty/zero fields → defaults,
	// plus storage-layer upper-bound clamps on chunk sizes).
	storage.NormalizeConfig(cfg)
	return cfg, nil
}

// SetConfig writes the drift configuration to the config file.
func (fs *FSStorage) SetConfig(ctx context.Context, config *core.Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(fs.root, ConfigFile)
	if err := fsutil.WriteFileAtomic(path, data, fsutil.DefaultFilePerm); err != nil {
		return fmt.Errorf("write config: %w", mapOSError(err))
	}
	return nil
}
