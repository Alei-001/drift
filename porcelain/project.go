package porcelain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
)

func InitProject(path string) error {
	ctx := context.Background()
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err == nil {
		return fmt.Errorf("already a drift repository")
	}

	store, err := filesystem.NewFSStorage(driftPath)
	if err != nil {
		return fmt.Errorf("create storage: %w", err)
	}
	defer store.Close()

	cfg := core.DefaultConfig()
	applyStorageConfig(store, &cfg.Core)

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

	driftignorePath := filepath.Join(path, ".driftignore")
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
	ctx := context.Background()
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err != nil {
		return nil, nil, fmt.Errorf("not a drift repository")
	}

	fsStore, err := filesystem.NewFSStorage(driftPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open storage: %w", err)
	}

	config, err := fsStore.GetConfig(ctx)
	if err != nil {
		fsStore.Close()
		return nil, nil, fmt.Errorf("get config: %w", err)
	}

	applyStorageConfig(fsStore, &config.Core)

	return fsStore, config, nil
}

func applyStorageConfig(store *filesystem.FSStorage, cfg *core.CoreConfig) {
	level := zstd.EncoderLevelFromZstd(cfg.ZstdLevel())
	store.SetCompression(cfg.Compression, level)
}
