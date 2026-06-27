package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	driftsync "github.com/drift/drift/internal/sync"
	"github.com/drift/drift/internal/worktree"
)

type SyncStatus struct {
	Enabled    bool
	RemoteName string
	LastSync   string
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
		return nil, fmt.Errorf("backup is not enabled (run 'drift backup on')")
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
		return nil, fmt.Errorf("backup is not enabled (run 'drift backup on')")
	}

	if branch == "" {
		branch = a.CurrentBranch()
	}

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

	if wip, _ := worktree.LoadWIP(a.store, branch); wip != nil && len(wip.Entries) > 0 {
		if _, err := a.RestoreWIP(branch); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to restore WIP: %v\n", err)
		}
	}

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
