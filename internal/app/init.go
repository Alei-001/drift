package app

import (
	"fmt"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/storage"
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

	// Generate a project ID for sync support and write the project config
	// once (core settings + project ID; user info comes from global config
	// unless overridden per-project).
	cfg := config.DefaultConfig()
	cfg.Sync.ProjectID = config.NewProjectID()
	if err := config.SaveConfig(a.store.DriftDir(), cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
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
