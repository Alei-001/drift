package porcelain

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/filesystem"
)

// InitProject initializes a new drift repository at the given path.
// Creates .drift/ directory structure, default config, HEAD reference, and empty index.
func InitProject(path string) error {
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err == nil {
		return fmt.Errorf("already a drift repository")
	}

	store, err := filesystem.NewFSStorage(driftPath)
	if err != nil {
		return fmt.Errorf("create storage: %w", err)
	}
	defer store.Close()

	if err := store.SetConfig(core.DefaultConfig()); err != nil {
		return fmt.Errorf("set config: %w", err)
	}

	headRef := &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: core.Hash{},
	}
	if err := store.SetRef("HEAD", headRef); err != nil {
		return fmt.Errorf("set HEAD: %w", err)
	}

	index := &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	}
	if err := store.SetIndex(index); err != nil {
		return fmt.Errorf("set index: %w", err)
	}

	return nil
}

// OpenProject opens an existing drift repository at the given path.
// Returns the storage backend, configuration, and any error.
func OpenProject(path string) (storage.Storer, *core.Config, error) {
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err != nil {
		return nil, nil, fmt.Errorf("not a drift repository")
	}

	store, err := filesystem.NewFSStorage(driftPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open storage: %w", err)
	}

	config, err := store.GetConfig()
	if err != nil {
		store.Close()
		return nil, nil, fmt.Errorf("get config: %w", err)
	}

	return store, config, nil
}
