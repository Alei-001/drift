// Package sync provides remote synchronization for drift projects.
//
// The sync engine supports multiple transports (local filesystem, WebDAV,
// FTP, SFTP, SMB) behind a common Transport interface. Synchronization is
// incremental (content-hash based) and tracks deletions via a manifest file
// stored on the remote. Auto-sync is triggered after 'drift save'.
package sync

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GlobalConfig stores drift-wide settings that apply across all projects.
// It lives at ~/.drift/global.json so it survives project cloning.
type GlobalConfig struct {
	// Protocol specifies the remote storage protocol.
	// Valid values: "local", "webdav", "ftp", "sftp", "smb".
	Protocol string `json:"protocol,omitempty"`

	// Host is the remote server hostname or IP (network protocols only).
	Host string `json:"host,omitempty"`

	// Port is the remote server port. If 0, the protocol default is used.
	Port int `json:"port,omitempty"`

	// Path is the remote base directory path, or the local filesystem path
	// for the "local" protocol.
	Path string `json:"path,omitempty"`

	// Username for authentication (network protocols).
	Username string `json:"username,omitempty"`

	// Password for authentication. Stored in plaintext; a future
	// improvement could use the OS keychain. Acceptable for self-hosted
	// NAS setups where the config file is on a trusted machine.
	Password string `json:"password,omitempty"`

	// TLS enables encrypted transport (FTPS for FTP, HTTPS for WebDAV).
	TLS bool `json:"tls,omitempty"`

	// InsecureSkipVerify disables TLS certificate verification. Useful for
	// self-signed certificates on self-hosted NAS servers. Only effective
	// when TLS is true.
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`

	// Share is the SMB share name (SMB protocol only).
	Share string `json:"share,omitempty"`

	// KeyPath is the path to a private key file for SSH authentication
	// (SFTP protocol only). If empty, password authentication is used.
	KeyPath string `json:"key_path,omitempty"`
}

// globalConfigPathOverride allows tests to redirect the global config to a
// temp directory. When empty, the default ~/.drift/global.json is used.
var globalConfigPathOverride string

// globalConfigPath returns the path to ~/.drift/global.json.
func globalConfigPath() (string, error) {
	if globalConfigPathOverride != "" {
		return globalConfigPathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".drift", "global.json"), nil
}

// SetGlobalConfigPathForTest overrides the global config path. Pass an empty
// string to restore the default. Intended for testing only.
func SetGlobalConfigPathForTest(path string) {
	globalConfigPathOverride = path
}

// LoadGlobalConfig reads the global config, returning an empty config if
// the file does not exist yet.
func LoadGlobalConfig() (*GlobalConfig, error) {
	path, err := globalConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return nil, fmt.Errorf("cannot read global config: %w", err)
	}
	cfg := &GlobalConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid global config: %w", err)
	}
	return cfg, nil
}

// SaveGlobalConfig writes the global config atomically.
func SaveGlobalConfig(cfg *GlobalConfig) error {
	path, err := globalConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	// Use 0600 so the plaintext password in the config is only readable by
	// the owner.
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NewProjectID generates a random 16-byte hex project identifier.
// Called once at 'drift init' time.
func NewProjectID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("fallback-%d", os.Getpid())
	}
	return hex.EncodeToString(b)
}

// RemoteType indicates what kind of remote is configured.
type RemoteType int

const (
	RemoteNone   RemoteType = iota
	RemoteLocal             // local filesystem (NAS mount, etc.)
	RemoteWebDAV            // WebDAV server
	RemoteFTP               // FTP server
	RemoteSFTP              // SFTP server (SSH file transfer)
	RemoteSMB               // SMB/CIFS share
)

// GetRemoteType returns the configured remote type based on the Protocol field.
func (g *GlobalConfig) GetRemoteType() RemoteType {
	switch g.Protocol {
	case "local":
		return RemoteLocal
	case "webdav":
		return RemoteWebDAV
	case "ftp":
		return RemoteFTP
	case "sftp":
		return RemoteSFTP
	case "smb":
		return RemoteSMB
	default:
		return RemoteNone
	}
}

// defaultPort returns the default port for a protocol.
// For webdav, the default depends on TLS (80 for HTTP, 443 for HTTPS),
// so callers should use EffectivePort which has access to the TLS flag.
func defaultPort(protocol string) int {
	switch protocol {
	case "ftp":
		return 21
	case "sftp":
		return 22
	case "smb":
		return 445
	case "webdav":
		return 80 // default; EffectivePort overrides to 443 when TLS is set
	default:
		return 0
	}
}

// EffectivePort returns the configured port or the protocol default.
// For webdav, the default port depends on the TLS flag (443 for HTTPS, 80 for HTTP).
func (g *GlobalConfig) EffectivePort() int {
	if g.Port != 0 {
		return g.Port
	}
	if g.Protocol == "webdav" && g.TLS {
		return 443
	}
	return defaultPort(g.Protocol)
}

// webDAVBaseURL reconstructs the WebDAV base URL from unified config fields.
func (g *GlobalConfig) webDAVBaseURL() string {
	scheme := "http"
	if g.TLS {
		scheme = "https"
	}
	basePath := strings.Trim(g.Path, "/")
	port := g.EffectivePort()
	if basePath != "" {
		return fmt.Sprintf("%s://%s:%d/%s", scheme, g.Host, port, basePath)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, g.Host, port)
}

// ProjectTransportForConfig returns a Transport scoped to a project, based
// on the global config. The caller must call Close() when done.
// Returns nil and an error if no remote is configured or connection fails.
func ProjectTransportForConfig(gcfg *GlobalConfig, remoteName string) (Transport, error) {
	switch gcfg.GetRemoteType() {
	case RemoteLocal:
		return NewLocalTransport(gcfg.Path).ProjectTransport(remoteName), nil
	case RemoteWebDAV:
		baseURL := gcfg.webDAVBaseURL() + "/" + remoteName
		return NewWebDAVTransport(baseURL, gcfg.Username, gcfg.Password, gcfg.InsecureSkipVerify), nil
	case RemoteFTP:
		return NewFTPTransport(gcfg, remoteName)
	case RemoteSFTP:
		return NewSFTPTransport(gcfg, remoteName)
	case RemoteSMB:
		return NewSMBTransport(gcfg, remoteName)
	}
	return nil, fmt.Errorf("no remote configured")
}

// LocalTransport implements sync over a local filesystem path (NAS mount,
// cloud-drive synced folder, USB drive, etc.). It implements the Transport
// interface for use with the sync Engine.
type LocalTransport struct {
	remoteRoot string
}

// NewLocalTransport creates a transport for the given remote root directory.
func NewLocalTransport(remoteRoot string) *LocalTransport {
	return &LocalTransport{remoteRoot: remoteRoot}
}

// Close releases any resources held by the transport. The local transport
// holds no resources, so this is a no-op.
func (t *LocalTransport) Close() error { return nil }

// RemoteRoot returns the configured remote root path.
func (t *LocalTransport) RemoteRoot() string {
	return t.remoteRoot
}

// remoteProjectDir returns the path to a project on the remote.
func (t *LocalTransport) remoteProjectDir(remoteName string) string {
	return filepath.Join(t.remoteRoot, remoteName)
}

// ProjectExists reports whether a project with the given remote name exists
// on the remote.
func (t *LocalTransport) ProjectExists(remoteName string) (bool, error) {
	dir := t.remoteProjectDir(remoteName)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// ListProjects returns the names of all projects on the remote.
func (t *LocalTransport) ListProjects() ([]string, error) {
	entries, err := os.ReadDir(t.remoteRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(t.remoteRoot, e.Name(), ".drift")); err == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Clone copies an entire remote project to a local destination directory.
func (t *LocalTransport) Clone(remoteName, destDir string) error {
	src := t.remoteProjectDir(remoteName)
	if exists, err := t.ProjectExists(remoteName); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("project %q not found on remote", remoteName)
	}

	if info, err := os.Stat(destDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("destination %q exists and is not a directory", destDir)
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("destination %q is not empty", destDir)
		}
	}

	return copyDir(src, destDir)
}

// ProjectTransport returns a Transport scoped to a specific project on the
// remote.
func (t *LocalTransport) ProjectTransport(remoteName string) Transport {
	return &localProjectTransport{
		root: filepath.Join(t.remoteRoot, remoteName),
	}
}

// localProjectTransport is a Transport scoped to a project directory.
type localProjectTransport struct {
	root string
}

func (t *localProjectTransport) abs(remotePath string) string {
	return filepath.Join(t.root, filepath.FromSlash(remotePath))
}

func (t *localProjectTransport) Get(remotePath string, dst io.Writer) error {
	f, err := os.Open(t.abs(remotePath))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return err
	}
	defer f.Close()
	_, err = io.Copy(dst, f)
	return err
}

func (t *localProjectTransport) Put(remotePath string, src io.Reader) error {
	abs := t.abs(remotePath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	tmp := abs + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, abs)
}

func (t *localProjectTransport) Stat(remotePath string) (*RemoteStat, error) {
	info, err := os.Stat(t.abs(remotePath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return nil, err
	}
	return &RemoteStat{
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

func (t *localProjectTransport) List(prefix string) ([]string, error) {
	root := t.root
	if prefix != "" {
		root = filepath.Join(t.root, filepath.FromSlash(prefix))
	}
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(t.root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func (t *localProjectTransport) Delete(remotePath string) error {
	err := os.Remove(t.abs(remotePath))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (t *localProjectTransport) Mkdir(remotePath string) error {
	return os.MkdirAll(t.abs(remotePath), 0755)
}

func (t *localProjectTransport) Close() error { return nil }

// copyDir recursively copies src into dst. Symlinks are skipped.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies a single file from src to dst, preserving mode.
func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(dst)
		return closeErr
	}
	return nil
}
