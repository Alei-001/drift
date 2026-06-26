// Package sync provides remote synchronization for drift projects.
//
// The sync engine supports multiple transports (local filesystem, WebDAV,
// FTP, SFTP, SMB) behind a common Transport interface. Synchronization is
// incremental (content-hash based) and tracks deletions via a manifest file
// stored on the remote. Auto-sync is triggered after 'drift save'.
package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/drift/drift/internal/config"
)

// RemoteType indicates what kind of remote is configured.
type RemoteType int

const (
	RemoteNone   RemoteType = iota
	RemoteLocal
	RemoteWebDAV
	RemoteFTP
	RemoteSFTP
	RemoteSMB
)

// GetRemoteType returns the configured remote type based on the Protocol field.
func GetRemoteType(g *config.GlobalConfig) RemoteType {
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

func defaultPort(protocol string) int {
	switch protocol {
	case "ftp":
		return 21
	case "sftp":
		return 22
	case "smb":
		return 445
	case "webdav":
		return 80
	default:
		return 0
	}
}

// EffectivePort returns the configured port or the protocol default.
func EffectivePort(g *config.GlobalConfig) int {
	if g.Port != 0 {
		return g.Port
	}
	if g.Protocol == "webdav" && g.TLS {
		return 443
	}
	return defaultPort(g.Protocol)
}

func webDAVBaseURL(g *config.GlobalConfig) string {
	scheme := "http"
	if g.TLS {
		scheme = "https"
	}
	basePath := strings.Trim(g.Path, "/")
	port := EffectivePort(g)
	if basePath != "" {
		return fmt.Sprintf("%s://%s:%d/%s", scheme, g.Host, port, basePath)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, g.Host, port)
}

// ProjectTransportForConfig returns a Transport scoped to a project, based
// on the global config. The caller must call Close() when done.
func ProjectTransportForConfig(gcfg *config.GlobalConfig, remoteName string) (Transport, error) {
	switch GetRemoteType(gcfg) {
	case RemoteLocal:
		return NewLocalTransport(gcfg.Path).ProjectTransport(remoteName), nil
	case RemoteWebDAV:
		baseURL := webDAVBaseURL(gcfg) + "/" + remoteName
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

func (t *LocalTransport) Close() error { return nil }

func (t *LocalTransport) RemoteRoot() string { return t.remoteRoot }

func (t *LocalTransport) remoteProjectDir(remoteName string) string {
	return filepath.Join(t.remoteRoot, remoteName)
}

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

func (t *LocalTransport) ProjectTransport(remoteName string) Transport {
	return &localProjectTransport{
		root: filepath.Join(t.remoteRoot, remoteName),
	}
}

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
