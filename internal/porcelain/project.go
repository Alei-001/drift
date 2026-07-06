package porcelain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/backends/filesystem"
)

// StoreFactory builds a storage.Storer rooted at the given .drift path.
// It is the seam used by InitProjectWithFactory / OpenProjectWithFactory to
// inject a non-default backend (e.g. the in-memory backend in tests).
type StoreFactory func(driftPath string) (storage.Storer, error)

func defaultStoreFactory(driftPath string) (storage.Storer, error) {
	return filesystem.NewFSStorage(driftPath)
}

// InitProject initializes a new drift repository at the given path using the
// default on-disk storage backend. It is a thin wrapper around
// InitProjectWithFactory with defaultStoreFactory.
func InitProject(path string) error {
	return InitProjectWithFactory(path, defaultStoreFactory)
}

// InitProjectWithFactory initializes a new drift repository at the given
// path, using the provided StoreFactory to construct the storage backend.
// The factory is the seam for injecting a non-default backend in tests.
func InitProjectWithFactory(path string, factory StoreFactory) error {
	ctx := context.Background()
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err == nil {
		return fmt.Errorf("already a drift repository")
	}

	store, err := factory(driftPath)
	if err != nil {
		return fmt.Errorf("create storage: %w", err)
	}
	defer store.Close()

	cfg := core.DefaultConfig()
	if err := applyStorageConfig(store, &cfg.Core); err != nil {
		return err
	}

	if err := store.SetConfig(ctx, cfg); err != nil {
		return fmt.Errorf("set config: %w", err)
	}

	mainRef := &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	}
	if err := store.SetRef(ctx, "heads/main", mainRef); err != nil {
		return fmt.Errorf("set main branch: %w", err)
	}

	headRef := &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	}
	if err := store.SetRef(ctx, "HEAD", headRef); err != nil {
		return fmt.Errorf("set HEAD: %w", err)
	}

	index := &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	}
	if err := store.SetIndex(ctx, index); err != nil {
		return fmt.Errorf("set index: %w", err)
	}

	driftignorePath := filepath.Join(path, core.DefaultIgnoreFile)
	if _, err := os.Stat(driftignorePath); os.IsNotExist(err) {
		driftignoreContent := []byte(`# macOS
.DS_Store

# Windows
Thumbs.db
desktop.ini

# Office temp files
~$*

# Editor temp files
*.tmp
*.swp
*~
`)
		if err := os.WriteFile(driftignorePath, driftignoreContent, 0644); err != nil {
			return fmt.Errorf("write .driftignore: %w", err)
		}
	}

	return nil
}

// OpenProject opens an existing drift repository at the given path using the
// default on-disk storage backend, returning the store, the loaded config,
// and any error. It is a thin wrapper around OpenProjectWithFactory with
// defaultStoreFactory.
func OpenProject(path string) (storage.Storer, *core.Config, error) {
	return OpenProjectWithFactory(path, defaultStoreFactory)
}

// OpenProjectWithFactory opens an existing drift repository at the given
// path, using the provided StoreFactory to construct the storage backend.
// The factory is the seam for injecting a non-default backend in tests.
func OpenProjectWithFactory(path string, factory StoreFactory) (storage.Storer, *core.Config, error) {
	ctx := context.Background()
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err != nil {
		return nil, nil, fmt.Errorf("not a drift repository")
	}

	store, err := factory(driftPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open storage: %w", err)
	}

	config, err := store.GetConfig(ctx)
	if err != nil {
		store.Close()
		return nil, nil, fmt.Errorf("get config: %w", err)
	}

	if err := applyStorageConfig(store, &config.Core); err != nil {
		store.Close()
		return nil, nil, err
	}

	return store, config, nil
}

// applyStorageConfig applies the core config's compression settings to the
// store via the ConfigStorer interface. This works for any backend that
// implements storage.Storer (filesystem, memory, future remote backends)
// without porcelain needing to type-assert to a concrete implementation.
func applyStorageConfig(store storage.Storer, cfg *core.CoreConfig) error {
	if err := store.SetCompressionConfig(cfg.Compression, cfg.ZstdLevel()); err != nil {
		return fmt.Errorf("apply storage config: %w", err)
	}
	return nil
}

// SetConfigValue parses value according to the key's type, validates ranges
// (compression.level 1-19, chunk sizes >= 0), writes it into cfg, and
// persists the updated config to storage. It returns an error if the key
// is unknown or the value is invalid.
func SetConfigValue(ctx context.Context, store storage.Storer, cfg *core.Config, key, value string) error {
	switch key {
	case "user.name":
		cfg.User.Name = value
	case "compression.enabled":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value '%s' for %s", value, key)
		}
		cfg.Core.Compression = b
	case "compression.level":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value '%s' for %s", value, key)
		}
		if n < core.MinZstdLevel || n > core.MaxZstdLevel {
			return fmt.Errorf("compression.level must be between %d and %d", core.MinZstdLevel, core.MaxZstdLevel)
		}
		cfg.Core.CompressionLevel = n
	case "chunk.min_size", "chunk.avg_size", "chunk.max_size":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer value '%s' for %s", value, key)
		}
		if n < 0 {
			return fmt.Errorf("%s must be non-negative", key)
		}
		switch key {
		case "chunk.min_size":
			cfg.Core.ChunkMinSize = n
		case "chunk.avg_size":
			cfg.Core.ChunkAvgSize = n
		case "chunk.max_size":
			cfg.Core.ChunkMaxSize = n
		}
	default:
		return fmt.Errorf("unknown config key '%s'", key)
	}

	if err := store.SetConfig(ctx, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
