package porcelain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/backends/filesystem"
)

type StoreFactory func(driftPath string) (storage.Storer, error)

func defaultStoreFactory(driftPath string) (storage.Storer, error) {
	return filesystem.NewFSStorage(driftPath)
}

func InitProject(path string) error {
	return InitProjectWithFactory(path, defaultStoreFactory)
}

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

func OpenProject(path string) (storage.Storer, *core.Config, error) {
	return OpenProjectWithFactory(path, defaultStoreFactory)
}

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

func applyStorageConfig(store storage.Storer, cfg *core.CoreConfig) error {
	fsStore, ok := store.(*filesystem.FSStorage)
	if !ok {
		return nil
	}
	level := zstd.EncoderLevelFromZstd(cfg.ZstdLevel())
	if err := fsStore.SetCompression(cfg.Compression, level); err != nil {
		return fmt.Errorf("apply storage config: %w", err)
	}
	return nil
}
