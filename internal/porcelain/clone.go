package porcelain

import (
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

	dir := opts.TargetDir
	if dir == "" {
		dir = path.Base(opts.RemoteURL)
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" || dir == "." || dir == string(filepath.Separator) {
			return nil, fmt.Errorf("cannot determine directory name from URL %q; specify one explicitly", opts.RemoteURL)
		}
	}
	targetDir := filepath.Join(opts.WorkDir, dir)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("create directory %q: %w", dir, err)
	}

	if err := InitProject(targetDir); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	store, _, err := OpenProject(targetDir)
	if err != nil {
		return nil, fmt.Errorf("open project: %w", err)
	}
	defer store.Close()

	rf, _ := remote.LoadRemotes(filepath.Join(targetDir, ".drift"))
	rf.AddOrUpdateRemote(remote.RemoteConfig{
		Name: "origin",
		Type: remoteType,
		URL:  opts.RemoteURL,
		User: opts.User,
	})
	if err := remote.SaveRemotes(filepath.Join(targetDir, ".drift"), rf); err != nil {
		return nil, err
	}

	credentialsSaved := false
	if opts.Password != "" {
		host, _ := remote.HostFromURL(opts.RemoteURL)
		cred, err := remote.LoadCredentials()
		if err != nil {
			return nil, fmt.Errorf("load credentials: %w", err)
		}
		cred.AddOrUpdateCredential(remote.Credential{
			Remote:   "origin",
			Host:     host,
			User:     opts.User,
			Password: opts.Password,
		})
		if err := remote.SaveCredentials(cred); err != nil {
			return nil, fmt.Errorf("save credentials: %w", err)
		}
		credentialsSaved = true
	}

	rCfg, err := resolveRemoteConfig(targetDir, "origin")
	if err != nil {
		return nil, fmt.Errorf("resolve remote: %w", err)
	}

	rfs, err := remote.NewRemoteFS(rCfg)
	if err != nil {
		return nil, fmt.Errorf("create remote client: %w", err)
	}
	defer rfs.Close()

	stats, err := remote.Pull(ctx, store, rfs, "")
	if err != nil {
		return nil, fmt.Errorf("pull: %w", err)
	}

	branchName := "main"
	if name := ResolveCurrentBranchName(ctx, store); name != "" {
		branchName = name
	}

	refs, _ := store.ListRefs(ctx, "")
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
