package porcelain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

// PushToRemote uploads local objects to the named remote. The remote must be
// configured in .drift/remotes.json with credentials in user-level
// credentials.json. If branch is non-empty, only that branch's snapshot chain
// and its chunks are pushed.
func PushToRemote(ctx context.Context, store storage.Storer, workDir, remoteName, branch string) (*PushResult, error) {
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

// PullFromRemote downloads remote objects to local. The remote must be
// configured in .drift/remotes.json with credentials in user-level
// credentials.json. If branch is non-empty, only that branch's snapshot chain
// and its chunks are pulled. After pulling, if the current branch tip changed,
// the local index is rebuilt and the working directory is left out of sync
// (the caller should advise the user to run 'drift restore').
func PullFromRemote(ctx context.Context, store storage.Storer, workDir, remoteName, branch string) (*PullResult, error) {
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
	host, err := remote.HostFromURL(cfg.URL)
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("parse remote URL: %w", err)
	}
	cred, err := remote.LoadCredentials()
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("load credentials: %w", err)
	}
	password, err := cred.FindCredential(host, cfg.User)
	if err != nil {
		return remote.RemoteConfig{}, fmt.Errorf("no credential for %s@%s: run 'drift remote add' to configure: %w", cfg.User, host, err)
	}
	if cfg.Options == nil {
		cfg.Options = make(map[string]string)
	}
	cfg.Options["_password"] = password
	return cfg, nil
}
