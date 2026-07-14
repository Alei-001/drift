package porcelain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

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
	return ResolveCurrentBranchName(ctx, store)
}

// resolveRemoteConfigWithWarn loads the remote config and sets the
// AllowInsecure flag based on DRIFT_ALLOW_INSECURE=1. The actual refusal
// of cleartext http happens in NewWebDAVFS (the protocol factory), not
// here — this centralizes the security check so new callers of
// NewRemoteFS cannot bypass it by forgetting to call IsInsecureScheme.
// When the user opts in, a warning is logged so the decision is visible.
func resolveRemoteConfigWithWarn(workDir, remoteName string) (remote.RemoteConfig, error) {
	cfg, err := resolveRemoteConfig(workDir, remoteName)
	if err != nil {
		return remote.RemoteConfig{}, err
	}
	cfg.AllowInsecure = allowInsecureRemote()
	if cfg.AllowInsecure && remote.IsInsecureScheme(cfg) {
		slog.Warn("remote URL uses insecure http scheme; credentials are sent in cleartext (DRIFT_ALLOW_INSECURE=1)",
			"remote", remoteName, "url", cfg.URL)
	}
	return cfg, nil
}

// allowInsecureRemote reports whether the user has opted into insecure http
// remotes via DRIFT_ALLOW_INSECURE=1. The opt-in is per-process (env var),
// not per-remote, so a single export applies to all subsequent sync calls.
func allowInsecureRemote() bool {
	return os.Getenv("DRIFT_ALLOW_INSECURE") == "1"
}

// parseConcurrency extracts the concurrency option from a RemoteConfig,
// returning 0 (default) when not set or invalid.
func parseConcurrency(cfg remote.RemoteConfig) int {
	if v, ok := cfg.Options["concurrency"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}

// PushToRemote uploads local objects to the named remote. It acquires the
// workspace lock for the duration of the push so that concurrent
// workspace-modifying operations (snapshot, switch, restore) do not observe
// an inconsistent local state while push is reading from it.
func PushToRemote(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*PushResult, error) {
	cfg, err := resolveRemoteConfigWithWarn(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, err
	}
	defer ReleaseWorkspaceLock(workDir)

	branch = resolveBranchOrDefault(ctx, store, branch, all)
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: create remote client: %w", remote.ErrNetwork, err)
	}
	defer rfs.Close()

	opts := remote.SyncOptions{Concurrency: parseConcurrency(cfg)}
	stats, err := remote.Push(ctx, store, rfs, branch, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", remote.ErrNetwork, err)
	}
	slog.Info("push completed", "remote", remoteName, "branch", branch, "snapshots", stats.SnapshotsUploaded, "chunks", stats.ChunksUploaded, "refs", stats.RefsUpdated)
	return &PushResult{Stats: stats}, nil
}

// PushDryRun returns what would be pushed without uploading. It acquires
// the workspace lock so the dry-run reflects a consistent local state.
func PushDryRun(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*remote.SyncStats, error) {
	cfg, err := resolveRemoteConfigWithWarn(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, err
	}
	defer ReleaseWorkspaceLock(workDir)

	branch = resolveBranchOrDefault(ctx, store, branch, all)
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: create remote client: %w", remote.ErrNetwork, err)
	}
	defer rfs.Close()

	opts := remote.SyncOptions{Concurrency: parseConcurrency(cfg)}
	stats, err := remote.PushDryRun(ctx, store, rfs, branch, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", remote.ErrNetwork, err)
	}
	return stats, nil
}

// PullFromRemote downloads remote objects to local. It acquires the
// workspace lock for the duration of the pull because pull writes to the
// local store and may rebuild the index.
func PullFromRemote(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*PullResult, error) {
	cfg, err := resolveRemoteConfigWithWarn(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, err
	}
	defer ReleaseWorkspaceLock(workDir)

	branch = resolveBranchOrDefault(ctx, store, branch, all)
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: create remote client: %w", remote.ErrNetwork, err)
	}
	defer rfs.Close()

	opts := remote.SyncOptions{Concurrency: parseConcurrency(cfg)}
	stats, err := remote.Pull(ctx, store, rfs, branch, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", remote.ErrNetwork, err)
	}
	slog.Info("pull completed", "remote", remoteName, "branch", branch, "snapshots", stats.SnapshotsUploaded, "chunks", stats.ChunksUploaded, "refs", stats.RefsUpdated, "index_rebuilt", stats.IndexRebuilt)
	return &PullResult{Stats: stats}, nil
}

// PullDryRun returns what would be pulled without downloading. It acquires
// the workspace lock so the dry-run reflects a consistent local state.
func PullDryRun(ctx context.Context, store storage.Storer, workDir, remoteName, branch string, all bool) (*remote.SyncStats, error) {
	cfg, err := resolveRemoteConfigWithWarn(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, err
	}
	defer ReleaseWorkspaceLock(workDir)

	branch = resolveBranchOrDefault(ctx, store, branch, all)
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: create remote client: %w", remote.ErrNetwork, err)
	}
	defer rfs.Close()

	opts := remote.SyncOptions{Concurrency: parseConcurrency(cfg)}
	stats, err := remote.PullDryRun(ctx, store, rfs, branch, opts)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", remote.ErrNetwork, err)
	}
	return stats, nil
}

// LsRemote lists all refs from a remote without downloading any objects.
func LsRemote(ctx context.Context, workDir, remoteName string) ([]*core.Reference, error) {
	cfg, err := resolveRemoteConfigWithWarn(workDir, remoteName)
	if err != nil {
		return nil, err
	}
	rfs, err := remote.NewRemoteFS(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: create remote client: %w", remote.ErrNetwork, err)
	}
	defer rfs.Close()

	refs, err := remote.LsRemote(ctx, rfs)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", remote.ErrNetwork, err)
	}
	return refs, nil
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
