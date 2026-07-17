package sync

import (
	"github.com/Alei-001/drift/internal/project"
	"github.com/Alei-001/drift/internal/branch"
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/remote"
)

// CloneOptions holds all parameters for CloneRemote.
type CloneOptions struct {
	TargetDir  string
	WorkDir    string
	RemoteURL  string
	RemoteType string
	User       string
	Password   string
	// RemoteName is the name under which the cloned remote is registered
	// in .drift/remotes.json and credentials.json. Defaults to "origin"
	// when empty. Allowing the caller to choose the name prevents the
	// second clone of the same URL with different credentials from
	// silently overwriting the first remote's stored password.
	RemoteName string
}

// CloneResult reports the outcome of a clone operation.
type CloneResult struct {
	Dir              string
	Snapshots        int
	Branches         int
	Tags             int
	Branch           string
	CredentialsSaved bool
}

// CloneRemote downloads a remote drift repository into a new directory.
func CloneRemote(ctx context.Context, opts CloneOptions) (*CloneResult, error) {
	remoteType := opts.RemoteType
	if remoteType == "" {
		remoteType = "webdav"
	}
	// Default the remote name to "origin" for backwards compatibility.
	// Callers that need to distinguish multiple remotes pointing at the
	// same URL with different credentials can set opts.RemoteName.
	remoteName := opts.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}

	// Refuse insecure http schemes unless explicitly opted in. This early
	// check is for user experience: it fails before creating the target
	// directory and initializing an empty repo. The security enforcement
	// also happens in NewWebDAVFS (checking cfg.AllowInsecure), so even
	// if a new caller forgets this check, the factory still refuses.
	if remote.IsInsecureScheme(remote.RemoteConfig{URL: opts.RemoteURL}) && !allowInsecureRemote() {
		return nil, fmt.Errorf("remote URL %q uses insecure http scheme; refusing to clone over an unencrypted connection. Set DRIFT_ALLOW_INSECURE=1 to override", opts.RemoteURL)
	}

	dir := opts.TargetDir
	if dir == "" {
		dir = path.Base(opts.RemoteURL)
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" || dir == "." || dir == string(filepath.Separator) {
			return nil, fmt.Errorf("cannot determine directory name from URL %q; specify one explicitly", opts.RemoteURL)
		}
	}
	// Reject directory names that escape the workspace or refer to an
	// absolute path. path.Base does not collapse "..", so a URL whose
	// final segment is ".." (e.g. "https://host/foo/..") yields dir="..",
	// which filepath.Join would resolve one level above WorkDir. Also
	// reject absolute paths that slipped through path.Base on platforms
	// where it does not strip a leading separator.
	dir = filepath.Clean(dir)
	if dir == ".." || strings.HasPrefix(dir, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("derived directory name %q escapes workspace; specify one explicitly", dir)
	}
	if filepath.IsAbs(dir) {
		return nil, fmt.Errorf("derived directory name %q is absolute; specify one explicitly", dir)
	}
	targetDir := filepath.Join(opts.WorkDir, dir)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create directory %q: %w", dir, err)
	}

	if err := project.InitProject(targetDir); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	store, _, err := project.OpenProject(targetDir)
	if err != nil {
		return nil, fmt.Errorf("open project: %w", err)
	}
	defer store.Close()

	rf, _ := remote.LoadRemotes(filepath.Join(targetDir, ".drift"))
	rf.AddOrUpdateRemote(remote.RemoteConfig{
		Name: remoteName,
		Type: remoteType,
		URL:  opts.RemoteURL,
		User: opts.User,
	})
	if err := remote.SaveRemotes(filepath.Join(targetDir, ".drift"), rf); err != nil {
		return nil, err
	}

	credentialsSaved := false
	if opts.Password != "" {
		host, err := remote.HostFromURL(opts.RemoteURL)
		if err != nil {
			return nil, fmt.Errorf("parse host from URL %q: %w", opts.RemoteURL, err)
		}
		cred, err := remote.LoadCredentials()
		if err != nil {
			return nil, fmt.Errorf("load credentials: %w", err)
		}
		cred.AddOrUpdateCredential(remote.Credential{
			Remote:   remoteName,
			Host:     host,
			User:     opts.User,
			Password: opts.Password,
		})
		if err := remote.SaveCredentials(cred); err != nil {
			return nil, fmt.Errorf("save credentials: %w", err)
		}
		credentialsSaved = true
	}

	rCfg, err := resolveRemoteConfig(targetDir, remoteName)
	if err != nil {
		return nil, fmt.Errorf("resolve remote: %w", err)
	}
	rCfg.AllowInsecure = allowInsecureRemote()

	rfs, err := remote.NewRemoteFS(rCfg)
	if err != nil {
		return nil, fmt.Errorf("%w: create remote client: %w", remote.ErrNetwork, err)
	}
	defer rfs.Close()

	stats, err := remote.Pull(ctx, store, rfs, "", remote.SyncOptions{Concurrency: parseConcurrency(rCfg)})
	if err != nil {
		return nil, fmt.Errorf("%w: pull: %w", remote.ErrNetwork, err)
	}

	branchName := "main"
	if name := branch.ResolveCurrentBranchName(ctx, store); name != "" {
		branchName = name
	}

	refs, _ := store.Refs.ListRefs(ctx, "")
	var branches, tags int
	for _, r := range refs {
		if r.Name == "HEAD" {
			continue
		}
		if strings.HasPrefix(r.Name, "heads/") {
			branches++
		} else if strings.HasPrefix(r.Name, "tags/") {
			tags++
		}
	}

	return &CloneResult{
		Dir:              dir,
		Snapshots:        stats.SnapshotsUploaded + stats.SnapshotsSkipped,
		Branches:         branches,
		Tags:             tags,
		Branch:           branchName,
		CredentialsSaved: credentialsSaved,
	}, nil
}
