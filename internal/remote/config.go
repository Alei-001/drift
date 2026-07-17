package remote

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Alei-001/drift/internal/util/fsutil"
)

// RemoteConfig describes a single configured remote. Password is NOT stored
// here — it lives in the user-level credentials.json, matched by remote name.
// Protocol-specific fields (SMB domain, S3 region/bucket, SFTP key path, etc.)
// go in Options so adding a new protocol never changes this struct.
//
// The "_password" key in Options is a runtime-only convention: the caller
// (resolveRemoteConfig) injects the password from credentials.json into
// Options["_password"] before passing the config to a ProtocolFactory.
// It must never be persisted to remotes.json or included in log output.
type RemoteConfig struct {
	Name    string            `json:"name"`
	Type    string            `json:"type"` // "webdav" | "smb" | future
	URL     string            `json:"url"`  // webdav: https://host[:port]/path; smb: smb://host[:port]/share[/path]
	User    string            `json:"user"`
	Options map[string]string `json:"options,omitempty"` // protocol-specific fields

	// AllowInsecure permits http:// URLs. It is a runtime-only flag
	// (never persisted to remotes.json) set by the porcelain layer based
	// on DRIFT_ALLOW_INSECURE=1. Protocol factories (e.g. NewWebDAVFS)
	// check this field and refuse cleartext http unless it is true. This
	// centralizes the security check in the constructor so new callers
	// cannot bypass it by forgetting to call IsInsecureScheme.
	AllowInsecure bool `json:"-"`
}

// RemotesFile is the on-disk format of .drift/remotes.json.
type RemotesFile struct {
	Remotes []RemoteConfig `json:"remotes"`
}

// LoadRemotes reads the remotes.json file from the given .drift directory.
// Returns an empty RemotesFile (not an error) when the file does not exist.
func LoadRemotes(driftDir string) (*RemotesFile, error) {
	path := filepath.Join(driftDir, "remotes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &RemotesFile{Remotes: []RemoteConfig{}}, nil
		}
		return nil, fmt.Errorf("read remotes.json: %w", err)
	}
	var rf RemotesFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parse remotes.json: %w", err)
	}
	if rf.Remotes == nil {
		rf.Remotes = []RemoteConfig{}
	}
	return &rf, nil
}

// SaveRemotes writes the remotes.json file to the given .drift directory.
// The write is atomic (temp file + rename).
func SaveRemotes(driftDir string, rf *RemotesFile) error {
	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal remotes.json: %w", err)
	}
	path := filepath.Join(driftDir, "remotes.json")
	// 0600: remotes.json contains remote URLs and usernames that could
	// leak infrastructure details (NAS addresses, server hostnames) to
	// other users on the system. Owner-only access matches the
	// credentials.json convention.
	return fsutil.WriteFileAtomic(path, data, 0o600)
}

// FindRemote returns the RemoteConfig with the given name, or an error
// wrapping os.ErrNotExist when no such remote is configured.
func (rf *RemotesFile) FindRemote(name string) (RemoteConfig, error) {
	for _, r := range rf.Remotes {
		if r.Name == name {
			return r, nil
		}
	}
	return RemoteConfig{}, fmt.Errorf("remote %q: %w", name, os.ErrNotExist)
}

// AddOrUpdateRemote adds r to the file, or replaces the existing entry with
// the same name. The returned bool is true when an existing entry was
// replaced.
func (rf *RemotesFile) AddOrUpdateRemote(r RemoteConfig) bool {
	for i, existing := range rf.Remotes {
		if existing.Name == r.Name {
			rf.Remotes[i] = r
			return true
		}
	}
	rf.Remotes = append(rf.Remotes, r)
	return false
}

// RemoveRemote deletes the remote with the given name. The returned bool is
// true when an entry was removed.
func (rf *RemotesFile) RemoveRemote(name string) bool {
	for i, existing := range rf.Remotes {
		if existing.Name == name {
			rf.Remotes = append(rf.Remotes[:i], rf.Remotes[i+1:]...)
			return true
		}
	}
	return false
}

// IsInsecureScheme returns true when cfg.URL uses http:// (cleartext).
// Production callers should refuse cleartext unless the user explicitly
// opts in via DRIFT_ALLOW_INSECURE=1. The check is exported so the
// porcelain and CLI layers can apply the same logic without importing
// protocol-specific packages.
func IsInsecureScheme(cfg RemoteConfig) bool {
	if cfg.URL == "" {
		return false
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return false
	}
	return u.Scheme == "http"
}
