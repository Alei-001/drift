package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/drift/drift/internal/worktree"
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
	if a.config == nil {
		return fmt.Errorf("repository config is not initialized")
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
	if a.config == nil {
		return fmt.Errorf("repository config is not initialized")
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

	engine := driftsync.NewEngine(transport, a.store)
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

	// Check for local changes before pulling. If dirty, save WIP first.
	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		return nil, err
	}
	currentCommit, err := a.currentCommit()
	if err != nil {
		return nil, fmt.Errorf("get current commit: %w", err)
	}
	dirty, err := a.wt.HasModifications(currentCommit, &idx, nil)
	if err != nil {
		return nil, fmt.Errorf("check modifications: %w", err)
	}
	if dirty {
		if err := a.wt.StageWorktreeChanges(&idx); err != nil {
			return nil, fmt.Errorf("capture worktree changes: %w", err)
		}
		if err := a.wt.SaveWIP(branch, &idx); err != nil {
			return nil, fmt.Errorf("save wip before pull: %w", err)
		}
		if err := a.store.SaveIndex(&core.Index{}); err != nil {
			_ = worktree.DeleteWIP(a.store, branch)
			return nil, fmt.Errorf("clear index: %w", err)
		}
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

	engine := driftsync.NewEngine(transport, a.store)
	result, err := engine.Pull(branch)
	if err != nil {
		return nil, err
	}
	if result.Pulled == 0 {
		return &PullStats{Branch: branch, Pulled: 0}, nil
	}

	// Restore working tree from the newly pulled commit.
	remoteHash, err := a.store.GetRef(branch)
	if err != nil {
		return nil, fmt.Errorf("get local ref: %w", err)
	}
	commit, err := a.store.GetCommit(remoteHash)
	if err != nil {
		return nil, fmt.Errorf("get pulled commit: %w", err)
	}
	tree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		return nil, fmt.Errorf("get tree: %w", err)
	}

	reader := core.NewTreeReader(a.store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return nil, err
	}

	targetPaths := make(map[string]bool)
	for _, b := range blobs {
		targetPaths[b.Path] = true
	}

	newIdx := &core.Index{}
	for _, b := range blobs {
		entry, err := a.wt.WriteBlob(b)
		if err != nil {
			return nil, err
		}
		if err := newIdx.Add(entry); err != nil {
			return nil, fmt.Errorf("add %s to index: %w", entry.Path, err)
		}
	}

	// Clean up files not in the pulled tree.
	var deletedPaths []string
	walkErr := core.WalkWorkingDir(a.dir, func(path string, info os.FileInfo) error {
		if targetPaths[path] {
			return nil
		}
		if err := core.ValidateTreePath(path); err != nil {
			return nil
		}
		fullPath := filepath.Join(a.dir, filepath.FromSlash(path))
		if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		deletedPaths = append(deletedPaths, path)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("clean worktree: %w", walkErr)
	}

	a.wt.CleanEmptyDirs(deletedPaths)

	if err := a.store.SaveIndex(newIdx); err != nil {
		return nil, fmt.Errorf("save index: %w", err)
	}

	// Restore WIP on top of pulled tree.
	if wip, _ := worktree.LoadWIP(a.store, branch); wip != nil && len(wip.Entries) > 0 {
		if _, err := a.RestoreWIP(branch); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to restore WIP: %v\n", err)
		}
	}

	// Update sync status.
	a.config.Sync.LastSync = time.Now().Format(time.RFC3339)
	if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
		return nil, fmt.Errorf("save sync status: %w", err)
	}

	return &PullStats{Branch: branch, Pulled: result.Pulled}, nil
}

type SyncStats struct {
	Pushed int
	Pulled int
}

func (a *App) SyncNow() (*SyncStats, error) {
	branch := a.CurrentBranch()
	pushStats, err := a.Push(branch)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	pullStats, err := a.Pull(branch)
	if err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}
	return &SyncStats{Pushed: pushStats.Pushed, Pulled: pullStats.Pulled}, nil
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

	engine := driftsync.NewEngine(transport, destStore)
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

	// Restore all files from the commit tree into the working directory.
	for _, b := range blobs {
		if err := core.ValidateTreePath(b.Path); err != nil {
			return 0, fmt.Errorf("unsafe path from remote %q: %w", b.Path, err)
		}
		targetPath := filepath.Join(destDir, filepath.FromSlash(b.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return 0, err
		}
		if b.Mode == core.ModeSymlink {
			data, err := destStore.GetBlob(b.Hash)
			if err != nil {
				return 0, fmt.Errorf("read symlink %s: %w", b.Path, err)
			}
			if err := os.Symlink(string(data), targetPath); err != nil {
				return 0, err
			}
		} else {
			data, err := destStore.GetBlob(b.Hash)
			if err != nil {
				return 0, fmt.Errorf("read blob %s: %w", b.Path, err)
			}
			if err := os.WriteFile(targetPath, data, os.FileMode(core.ToOSFileMode(b.Mode))); err != nil {
				return 0, err
			}
		}
	}

	// Build and save the index so drift status works immediately.
	idx := &core.Index{}
	for _, b := range blobs {
		entry := core.IndexEntry{
			Path: b.Path,
			Hash: b.Hash,
			Mode: b.Mode,
		}
		if err := idx.Add(entry); err != nil {
			return 0, fmt.Errorf("add %s to index: %w", entry.Path, err)
		}
	}
	if err := destStore.SaveIndex(idx); err != nil {
		return 0, fmt.Errorf("save index: %w", err)
	}

	return len(blobs), nil
}

func getRefByPrefix(store localStore, prefix string) string {
	refs, err := store.ListRefs()
	if err != nil {
		return ""
	}
	for name, hash := range refs {
		if strings.HasPrefix(name, prefix) {
			return hash
		}
	}
	return ""
}

// localStore mirrors sync's localStore for compile-time compatibility.
type localStore interface {
	GetRef(name string) (string, error)
	SaveRef(name string, hash string) error
	ListRefs() (map[string]string, error)
	GetCommit(hash string) (*core.Commit, error)
	GetTree(hash string) (*core.Tree, error)
}
