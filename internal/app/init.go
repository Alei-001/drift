package app

import (
	"fmt"
	"os"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/storage"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/drift/drift/internal/worktree"
)

func (a *App) Init() error {
	if err := a.store.Init(); err != nil {
		return err
	}
	if err := a.store.SaveRef("main", ""); err != nil {
		return fmt.Errorf("failed to create main branch: %w", err)
	}
	if err := a.store.SaveRef("HEAD", "main"); err != nil {
		return fmt.Errorf("failed to set HEAD: %w", err)
	}

	// Generate a project ID for sync support.
	cfg := config.DefaultConfig()
	cfg.Sync.ProjectID = driftsync.NewProjectID()

	// Save project config (core settings + project ID; user info
	// comes from global config unless overridden per-project).
	if err := config.SaveConfig(a.store.DriftDir(), cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save config: %v\n", err)
	}
	a.config = cfg

	return nil
}

func (a *App) IsInitialized() bool {
	return a.store.IsInitialized()
}

func (a *App) Chdir(dir string) error {
	a.store = storage.NewStore(dir)
	cfg, err := config.LoadConfig(a.store.DriftDir())
	if err != nil {
		return err
	}
	a.config = cfg
	autoCRLF := ""
	if cfg != nil {
		autoCRLF = cfg.Core.AutoCRLF
	}
	a.wt = worktree.New(a.store, dir, autoCRLF)
	a.dir = dir
	return nil
}
