package porcelain

import (
	"context"
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

	if err := store.SetConfig(ctx, core.DefaultConfig()); err != nil {
		return fmt.Errorf("set config: %w", err)
	}

	// Create main branch ref (zero hash = no commits yet)
	mainRef := &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	}
	if err := store.SetRef(ctx, "heads/main", mainRef); err != nil {
		return fmt.Errorf("set main branch: %w", err)
	}

	// Create HEAD as a symbolic reference pointing to heads/main
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

	// Create default .driftignore in project root if it doesn't exist
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

// OpenProject opens an existing drift repository at the given path.
// Returns the storage backend, configuration, and any error.
func OpenProject(path string) (storage.Storer, *core.Config, error) {
	ctx := context.Background()
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err != nil {
		return nil, nil, fmt.Errorf("not a drift repository")
	}

	store, err := filesystem.NewFSStorage(driftPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open storage: %w", err)
	}

	config, err := store.GetConfig(ctx)
	if err != nil {
		store.Close()
		return nil, nil, fmt.Errorf("get config: %w", err)
	}

	return store, config, nil
}
