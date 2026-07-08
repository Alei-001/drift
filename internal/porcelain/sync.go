package porcelain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/remote"
	"github.com/Alei-001/drift/internal/storage"
)

// PushResult reports the outcome of a push operation.
type PushResult struct {
	Stats *remote.SyncStats
}

// PullResult reports the outcome of a pull operation.
type PullResult struct {
	Stats *remote.SyncStats
}

// resolveBranchOrDefault returns branch name from a flag value. When branch
// is empty and all is false, resolves the current branch name. When all is
// true, returns "" (push/pull all).
func resolveBranchOrDefault(ctx context.Context, store storage.Storer, branch string, all bool) string {
	if all || branch != "" {
		return branch
	}
	if name, err := remote.CurrentBranchName(ctx, store); err == nil {
		return strings.TrimPrefix(name, "heads/")
	}
	return ""
}

// PushToRemote uploads local objects to the named remote.
func PushToRemote(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*PushResult, error) {
	branch = resolveBranchOrDefault(ctx, store, branch, all)
	cfg, err := resolveRemoteConfig(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("create remote client: %w", err)
	}
	defer rfs.Close()

	stats, err := remote.Push(ctx, store, rfs, branch)
	if err != nil {
		return nil, err
	}
	return &PushResult{Stats: stats}, nil
}

// PushDryRun returns what would be pushed without uploading.
func PushDryRun(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*remote.SyncStats, error) {
	branch = resolveBranchOrDefault(ctx, store, branch, all)
	cfg, err := resolveRemoteConfig(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("create remote client: %w", err)
	}
	defer rfs.Close()

	return remote.PushDryRun(ctx, store, rfs, branch)
}

// PullFromRemote downloads remote objects to local.
func PullFromRemote(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*PullResult, error) {
	branch = resolveBranchOrDefault(ctx, store, branch, all)
	cfg, err := resolveRemoteConfig(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("create remote client: %w", err)
	}
	defer rfs.Close()

	stats, err := remote.Pull(ctx, store, rfs, branch)
	if err != nil {
		return nil, err
	}
	return &PullResult{Stats: stats}, nil
}

// PullDryRun returns what would be pulled without downloading.
func PullDryRun(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*remote.SyncStats, error) {
	branch = resolveBranchOrDefault(ctx, store, branch, all)
	cfg, err := resolveRemoteConfig(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("create remote client: %w", err)
	}
	defer rfs.Close()

	return remote.PullDryRun(ctx, store, rfs, branch)
}

// LsRemote lists all refs from a remote without downloading any objects.
func LsRemote(ctx context.Context, workDir, remoteName string) ([]*core.Reference, error) {
	cfg, err := resolveRemoteConfig(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("create remote client: %w", err)
	}
	defer rfs.Close()

	return remote.LsRemote(ctx, rfs)
}

// resolveRemoteConfig loads the remote definition from .drift/remotes.json and
// merges the password from user-level credentials.json. The password is stashed
// in cfg.Options["_password"] for the protocol factory to read.
func resolveRemoteConfig(workDir, remoteName string) (remote.RemoteConfig, error) {
	driftDir := filepath.Join(workDir, ".drift")
	if _, err := os.Stat(driftDir); err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("not a drift repository: %w", err)
	}
	rf, err := remote.LoadRemotes(driftDir)
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("load remotes: %w", err)
	}
	cfg, err := rf.FindRemote(remoteName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return remote.RemoteConfig{}, fmt.Errorf("remote %q not found", remoteName)
		}
		return remote.RemoteConfig{}, fmt.Errorf("find remote %q: %w", remoteName, err)
	}
	cred, err := remote.LoadCredentials()
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("load credentials: %w", err)
	}
	password, err := cred.FindCredential(remoteName)
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("no credential for remote %q: run 'drift remote add' to configure: %w", remoteName, err)
	}
	if cfg.Options == nil {
		cfg.Options = make(map[string]string)
	}
	cfg.Options["_password"] = password
	return cfg, nil
}
