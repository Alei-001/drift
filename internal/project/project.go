package project

import (
	"github.com/Alei-001/drift/internal/errs"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/filesystem"
)

// StoreFactory builds a *store.StoreSet rooted at the given .drift path.
// It is the seam used by InitProjectWithFactory / OpenProjectWithFactory to
// inject a non-default backend (e.g. the in-memory backend in tests).
type StoreFactory func(driftPath string) (*store.StoreSet, error)

func defaultStoreFactory(driftPath string) (*store.StoreSet, error) {
	fs, err := filesystem.NewFSStorage(driftPath)
	if err != nil {
		return nil, err
	}
	return store.NewStoreSet(fs), nil
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

	st, err := factory(driftPath)
	if err != nil {
		return fmt.Errorf("create storage: %w", err)
	}
	defer st.Close()

	cfg := core.DefaultConfig()
	if err := applyStorageConfig(st, &cfg.Core); err != nil {
		return err
	}

	if err := st.Config.SetConfig(ctx, cfg); err != nil {
		return fmt.Errorf("set config: %w", err)
	}

	mainRef := &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	}
	if err := st.Refs.SetRef(ctx, "heads/main", mainRef); err != nil {
		return fmt.Errorf("set main branch: %w", err)
	}

	headRef := &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	}
	if err := st.Refs.SetRef(ctx, "HEAD", headRef); err != nil {
		return fmt.Errorf("set HEAD: %w", err)
	}

	index := &core.Index{
		Entries:   []core.IndexEntry{},
		UpdatedAt: time.Now().Unix(),
	}
	if err := st.Index.SetIndex(ctx, index); err != nil {
		return fmt.Errorf("set index: %w", err)
	}

	// Note: .driftignore is NOT auto-created. Users add ignore rules
	// on demand via 'drift ignore add <pattern>', which creates the
	// file when it does not yet exist. This mirrors git's .gitignore
	// behavior and avoids polluting the first snapshot with an empty
	// ignore file. See cli-design.md and AddIgnoreRules in fsutil.

	return nil
}

// OpenProject opens an existing drift repository at the given path using the
// default on-disk storage backend, returning the st, the loaded config,
// and any error. It is a thin wrapper around OpenProjectWithFactory with
// defaultStoreFactory.
func OpenProject(path string) (*store.StoreSet, *core.Config, error) {
	return OpenProjectWithFactory(path, defaultStoreFactory)
}

// OpenProjectWithFactory opens an existing drift repository at the given
// path, using the provided StoreFactory to construct the storage backend.
// The factory is the seam for injecting a non-default backend in tests.
func OpenProjectWithFactory(path string, factory StoreFactory) (*store.StoreSet, *core.Config, error) {
	ctx := context.Background()
	driftPath := filepath.Join(path, ".drift")

	if _, err := os.Stat(driftPath); err != nil {
		return nil, nil, fmt.Errorf("%w", errs.ErrNotARepo)
	}

	st, err := factory(driftPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open storage: %w", err)
	}

	config, err := st.Config.GetConfig(ctx)
	if err != nil {
		st.Close()
		return nil, nil, fmt.Errorf("get config: %w", err)
	}

	if err := applyStorageConfig(st, &config.Core); err != nil {
		st.Close()
		return nil, nil, err
	}

	return st, config, nil
}

// applyStorageConfig applies the core config's compression settings to the
// store via the ConfigStorer interface. This works for any backend that
// implements *store.StoreSet (filesystem, memory, future remote backends)
// without porcelain needing to type-assert to a concrete implementation.
func applyStorageConfig(st *store.StoreSet, cfg *core.CoreConfig) error {
	if err := st.Config.SetCompressionConfig(cfg.Compression, cfg.ZstdLevel()); err != nil {
		return fmt.Errorf("apply storage config: %w", err)
	}
	return nil
}

// SetConfigValue writes a user-configurable key into cfg and persists the
// updated config to store. Only user-facing keys (user.name, user.email)
// are accepted; algorithm tuning parameters (chunk sizes, compression) are
// intentionally not exposed — they are hardcoded in core.DefaultConfig and
// should not be tuned by end users. Returns an error if the key is unknown.
func SetConfigValue(ctx context.Context, st *store.StoreSet, cfg *core.Config, key, value string) error {
	switch key {
	case "user.name":
		cfg.User.Name = value
	case "user.email":
		cfg.User.Email = value
	default:
		return fmt.Errorf("unknown config key '%s'", key)
	}

	if err := st.Config.SetConfig(ctx, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
