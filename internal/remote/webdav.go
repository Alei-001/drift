package remote

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

// init registers the webdav protocol factory.
func init() {
	Register("webdav", NewWebDAVFS)
}

// WebDAVFS implements RemoteFS over the WebDAV protocol using gowebdav.
// The underlying *gowebdav.Client is safe for concurrent use.
type WebDAVFS struct {
	client *gowebdav.Client
	// rootPath is the base path on the WebDAV server (the path component
	// of the configured URL). All operations are relative to this root.
	rootPath string
}

// NewWebDAVFS constructs a WebDAVFS from a RemoteConfig. The URL must be a
// full HTTP/HTTPS WebDAV endpoint (e.g. https://nas.example.com/dav/drift).
// The password is read from cfg.Options["_password"] (populated by
// resolveRemoteConfig from the user-level credentials.json).
func NewWebDAVFS(cfg RemoteConfig) (RemoteFS, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("webdav: URL is required")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("webdav: parse URL %q: %w", cfg.URL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("webdav: URL scheme must be http or https, got %q", u.Scheme)
	}
	password := cfg.Options["_password"]

	// gowebdav NewClient takes the full endpoint URL.
	client := gowebdav.NewClient(cfg.URL, cfg.User, password)
	// Use a reasonable timeout for PROPFIND/GET/PUT operations.
	client.SetTimeout(60 * time.Second)

	return &WebDAVFS{
		client:   client,
		rootPath: "/",
	}, nil
}

// resolve joins a relative path with the root. We keep paths as-is (relative
// to the server root) since gowebdav operates on absolute paths.
func (w *WebDAVFS) resolve(p string) string {
	if p == "" || p == "." {
		return w.rootPath
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// Stat returns metadata for a remote path.
func (w *WebDAVFS) Stat(p string) (*RemoteInfo, error) {
	info, err := w.client.Stat(w.resolve(p))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("stat %q: %w", p, os.ErrNotExist)
		}
		return nil, fmt.Errorf("stat %q: %w", p, err)
	}
	return &RemoteInfo{
		Path:    p,
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime(),
	}, nil
}

// Read opens a remote file for streaming. The caller must close the reader.
func (w *WebDAVFS) Read(p string) (io.ReadCloser, error) {
	rc, err := w.client.ReadStream(w.resolve(p))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("read %q: %w", p, os.ErrNotExist)
		}
		return nil, fmt.Errorf("read %q: %w", p, err)
	}
	return rc, nil
}

// Write uploads a file, creating parent directories as needed.
func (w *WebDAVFS) Write(p string, r io.Reader) error {
	resolved := w.resolve(p)
	// Ensure parent directory exists.
	parent := path.Dir(resolved)
	if parent != "/" && parent != "." {
		if err := w.client.MkdirAll(parent, 0o755); err != nil {
			// MkdirAll may fail if the dir already exists; ignore that case.
			if !gowebdav.IsErrCode(err, 405) {
				return fmt.Errorf("mkdir parent %q: %w", parent, err)
			}
		}
	}
	if err := w.client.WriteStream(resolved, r, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", p, err)
	}
	return nil
}

// Remove deletes a remote file. A missing file is not an error.
func (w *WebDAVFS) Remove(p string) error {
	if err := w.client.Remove(w.resolve(p)); err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", p, err)
	}
	return nil
}

// List enumerates entries directly under a directory path (non-recursive).
func (w *WebDAVFS) List(p string) ([]RemoteInfo, error) {
	infos, err := w.client.ReadDir(w.resolve(p))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return []RemoteInfo{}, nil
		}
		return nil, fmt.Errorf("list %q: %w", p, err)
	}
	result := make([]RemoteInfo, 0, len(infos))
	for _, info := range infos {
		childPath := path.Join(w.resolve(p), info.Name())
		result = append(result, RemoteInfo{
			Path:    childPath,
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime(),
		})
	}
	return result, nil
}

// MkdirAll creates a directory tree.
func (w *WebDAVFS) MkdirAll(p string) error {
	if err := w.client.MkdirAll(w.resolve(p), 0o755); err != nil {
		// 405 Method Not Allowed is returned when the dir already exists.
		if !gowebdav.IsErrCode(err, 405) {
			return fmt.Errorf("mkdirall %q: %w", p, err)
		}
	}
	return nil
}

// Close is a no-op for WebDAV (stateless HTTP).
func (w *WebDAVFS) Close() error { return nil }

// Ensure WebDAVFS satisfies RemoteFS at compile time.
var _ RemoteFS = (*WebDAVFS)(nil)
