package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	driftsync "github.com/drift/drift/internal/sync"
)

type SyncStatus struct {
	Enabled    bool
	RemoteName string
	LastSync   string
}

type SyncRemoteOptions struct {
	Host               string
	Port               int
	Path               string
	Username           string
	Password           string
	TLS                bool
	InsecureSkipVerify bool
	Share              string
	KeyPath            string
}

type SyncRemoteInfo struct {
	Protocol string
	Host     string
	Port     int
	Path     string
	Username string
	TLS      bool
	Share    string
}

func (a *App) SyncEnable() error {
	if !a.IsInitialized() {
		return fmt.Errorf("not a drift repository")
	}

	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	if gcfg.Protocol == "" {
		return fmt.Errorf("no remote configured (run 'drift config remote --protocol <...>' first)")
	}

	remoteName := filepath.Base(a.dir)

	if a.config.Sync.ProjectID == "" {
		a.config.Sync.ProjectID = config.NewProjectID()
	}
	a.config.Sync.Enabled = true
	a.config.Sync.RemoteName = remoteName

	if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

func (a *App) SyncDisable() error {
	if !a.IsInitialized() {
		return fmt.Errorf("not a drift repository")
	}

	a.config.Sync.Enabled = false
	if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

type PushStats struct {
	Branch string
	Pushed int
}

func (a *App) Push(branch string) (*PushStats, error) {
	if !a.IsInitialized() {
		return nil, fmt.Errorf("not a drift repository")
	}

	if !a.checkSyncEnabled() {
		return nil, fmt.Errorf("sync is not enabled (run 'drift sync enable')")
	}

	if branch == "" {
		branch = a.CurrentBranch()
	}

	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}

	transport, err := driftsync.CreateTransport(gcfg, a.config.Sync.RemoteName)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote: %w", err)
	}
	defer transport.Close()

	engine := driftsync.NewEngine(transport, a.store, a.dir)
	result, err := engine.Push(branch)
	if err != nil {
		return nil, err
	}

	return &PushStats{Branch: result.Branch, Pushed: result.Pushed}, nil
}

type PullStats struct {
	Branch string
	Pulled int
}

func (a *App) Pull(branch string) (*PullStats, error) {
	if !a.IsInitialized() {
		return nil, fmt.Errorf("not a drift repository")
	}

	if !a.checkSyncEnabled() {
		return nil, fmt.Errorf("sync is not enabled (run 'drift sync enable')")
	}

	if branch == "" {
		branch = a.CurrentBranch()
	}

	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}

	transport, err := driftsync.CreateTransport(gcfg, a.config.Sync.RemoteName)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote: %w", err)
	}
	defer transport.Close()

	engine := driftsync.NewEngine(transport, a.store, a.dir)
	result, err := engine.Pull(branch)
	if err != nil {
		return nil, err
	}

	return &PullStats{Branch: result.Branch, Pulled: result.Pulled}, nil
}

func (a *App) SyncNow() (*PullStats, error) {
	// Sync = push first, then pull.
	branch := a.CurrentBranch()
	if stats, err := a.Push(branch); err != nil {
		return nil, fmt.Errorf("push: %w", err)
	} else if stats.Pushed > 0 {
		// Already pushed; continue to pull.
	}
	return a.Pull(branch)
}

func (a *App) checkSyncEnabled() bool {
	return a.IsInitialized() && a.config.Sync.Enabled
}

func (a *App) SyncStatus() (*SyncStatus, error) {
	if !a.IsInitialized() {
		return nil, fmt.Errorf("not a drift repository")
	}

	return &SyncStatus{
		Enabled:    a.config.Sync.Enabled,
		RemoteName: a.config.Sync.RemoteName,
		LastSync:   a.config.Sync.LastSync,
	}, nil
}

func (a *App) SyncEnabled() bool {
	return a.IsInitialized() && a.config.Sync.Enabled
}

func (a *App) AutoSync() error {
	if !a.SyncEnabled() {
		return nil
	}
	_, err := a.SyncNow()
	return err
}

func (a *App) SyncRemoteSet(protocol string, opts SyncRemoteOptions) error {
	gcfg := &config.GlobalConfig{
		Protocol:           protocol,
		Host:               opts.Host,
		Port:               opts.Port,
		Path:               opts.Path,
		Username:           opts.Username,
		Password:           opts.Password,
		TLS:                opts.TLS,
		InsecureSkipVerify: opts.InsecureSkipVerify,
		Share:              opts.Share,
		KeyPath:            opts.KeyPath,
	}

	return config.SaveGlobalConfig(gcfg)
}

func (a *App) SyncRemoteShow() (*SyncRemoteInfo, error) {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}
	if gcfg.Protocol == "" {
		return nil, fmt.Errorf("no remote configured")
	}
	return &SyncRemoteInfo{
		Protocol: gcfg.Protocol,
		Host:     gcfg.Host,
		Port:     gcfg.Port,
		Path:     gcfg.Path,
		Username: gcfg.Username,
		TLS:      gcfg.TLS,
		Share:    gcfg.Share,
	}, nil
}

func (a *App) SyncRemoteUnset() error {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	gcfg.Protocol = ""
	gcfg.Host = ""
	gcfg.Port = 0
	gcfg.Path = ""
	gcfg.Username = ""
	gcfg.Password = ""
	gcfg.TLS = false
	gcfg.InsecureSkipVerify = false
	gcfg.Share = ""
	gcfg.KeyPath = ""
	if err := config.SaveGlobalConfig(gcfg); err != nil {
		return err
	}

	if a.IsInitialized() && a.config.Sync.Enabled {
		a.config.Sync.Enabled = false
		if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
			return fmt.Errorf("failed to disable sync: %w", err)
		}
	}

	return nil
}

func (a *App) Clone(remoteName, destDir string) (int, error) {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return 0, err
	}
	if gcfg.Protocol == "" {
		return 0, fmt.Errorf("no remote configured (run 'drift config remote --protocol <...>' first)")
	}

	if !filepath.IsAbs(destDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return 0, err
		}
		destDir = filepath.Join(cwd, destDir)
	}

	if info, err := os.Stat(destDir); err == nil {
		if !info.IsDir() {
			return 0, fmt.Errorf("destination %q exists and is not a directory", destDir)
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			return 0, err
		}
		if len(entries) > 0 {
			return 0, fmt.Errorf("destination %q is not empty", destDir)
		}
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, err
	}

	transport, err := driftsync.CreateTransport(gcfg, remoteName)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to remote: %w", err)
	}
	defer transport.Close()

	destStore := storage.NewStore(destDir)
	if err := destStore.Init(); err != nil {
		return 0, fmt.Errorf("init store: %w", err)
	}

	engine := driftsync.NewEngine(transport, destStore, destDir)
	if err := engine.Clone(); err != nil {
		return 0, fmt.Errorf("clone failed: %w", err)
	}

	// Find default branch from tracking refs.
	mainHash := getRefByPrefix(destStore, "remotes/origin/heads/main")
	if mainHash == "" {
		mainHash = getRefByPrefix(destStore, "remotes/origin/main")
	}
	if mainHash == "" {
		return 0, fmt.Errorf("no main branch found on remote")
	}

	if err := destStore.SaveRef("main", mainHash); err != nil {
		return 0, fmt.Errorf("set main ref: %w", err)
	}
	if err := destStore.SaveRef("HEAD", "main"); err != nil {
		return 0, fmt.Errorf("set HEAD: %w", err)
	}

	commit, err := destStore.GetCommit(mainHash)
	if err != nil {
		return 0, fmt.Errorf("get commit: %w", err)
	}
	tree, err := destStore.GetTree(commit.TreeHash)
	if err != nil {
		return 0, err
	}

	reader := core.NewTreeReader(destStore)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return 0, err
	}

	return len(blobs), nil
}

func getRefByPrefix(store localStore, prefix string) string {
	refs, err := store.ListRefs()
	if err != nil {
		return ""
	}
	for name, hash := range refs {
		if stringHasPrefix(name, prefix) {
			return hash
		}
	}
	return ""
}

func stringHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// localStore mirrors sync's localStore for compile-time compatibility.
type localStore interface {
	GetRef(name string) (string, error)
	SaveRef(name string, hash string) error
	ListRefs() (map[string]string, error)
	GetCommit(hash string) (*core.Commit, error)
	GetTree(hash string) (*core.Tree, error)
}
