package remote

import (
	"context"
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
	// rootPath is the default path returned by resolve for empty inputs.
	// gowebdav's NewClient receives the full endpoint URL and uses its path
	// component as the base for all requests, so "/" is correct here —
	// operations are already scoped to the URL's path by gowebdav.
	rootPath string
}

// IsInsecureScheme reports whether the configured remote URL uses an
// unencrypted scheme (http) over which credentials would be sent in
// cleartext. The porcelain layer calls this after resolving the config to
// surface a user-facing warning; the remote layer itself never writes to
// stderr. smb:// and https:// are considered secure.
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

// NewWebDAVFS constructs a WebDAVFS from a RemoteConfig. The URL must be a
// full HTTP/HTTPS WebDAV endpoint (e.g. https://nas.example.com/dav/drift).
// The password is read from cfg.Options["_password"] (populated by
// resolveRemoteConfig from the user-level credentials.json).
//
// An http:// URL is accepted (so local test servers and trusted LANs work)
// but callers should check IsInsecureScheme and warn the user that the
// password is sent in cleartext.
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
func (w *WebDAVFS) Stat(ctx context.Context, p string) (*RemoteInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
func (w *WebDAVFS) Read(ctx context.Context, p string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rc, err := w.client.ReadStream(w.resolve(p))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("read %q: %w", p, os.ErrNotExist)
		}
		return nil, fmt.Errorf("read %q: %w", p, err)
	}
	return rc, nil
}

// mkdirParents ensures the parent directory of resolved exists. It stats
// first so the common "already exists" case avoids a failing MkdirAll whose
// 405 error code is server-dependent and unreliable as an existence signal.
func (w *WebDAVFS) mkdirParents(ctx context.Context, resolved string) error {
	parent := path.Dir(resolved)
	if parent == "/" || parent == "." {
		return nil
	}
	if _, err := w.client.Stat(parent); err == nil {
		return nil // parent exists
	} else if !gowebdav.IsErrNotFound(err) {
		return fmt.Errorf("stat parent %q: %w", parent, err)
	}
	if err := w.client.MkdirAll(parent, 0o755); err != nil {
		// A concurrent create may have made the dir appear between our
		// Stat and MkdirAll; re-stat to confirm before failing.
		if _, sErr := w.client.Stat(parent); sErr == nil {
			return nil
		}
		return fmt.Errorf("mkdir parent %q: %w", parent, err)
	}
	return nil
}

// Write uploads a file atomically: it first writes to <path>.partial, then
// renames it onto the final path. This prevents a network interruption from
// leaving a half-written object on the remote that other clients would
// mistake for a complete content-addressed object. gowebdav's Rename
// supports overwrite so the final path is replaced cleanly.
func (w *WebDAVFS) Write(ctx context.Context, p string, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved := w.resolve(p)
	if err := w.mkdirParents(ctx, resolved); err != nil {
		return err
	}
	partial := resolved + ".partial"
	if err := w.client.WriteStream(partial, r, 0o644); err != nil {
		// Best-effort cleanup of the partial upload so it does not linger.
		_ = w.client.Remove(partial)
		return fmt.Errorf("write partial %q: %w", p, err)
	}
	if err := w.client.Rename(partial, resolved, true); err != nil {
		_ = w.client.Remove(partial)
		return fmt.Errorf("rename partial to %q: %w", p, err)
	}
	return nil
}

// Remove deletes a remote file. A missing file is not an error.
func (w *WebDAVFS) Remove(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := w.client.Remove(w.resolve(p)); err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", p, err)
	}
	return nil
}

// List enumerates entries directly under a directory path (non-recursive).
func (w *WebDAVFS) List(ctx context.Context, p string) ([]RemoteInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolved := w.resolve(p)
	infos, err := w.client.ReadDir(resolved)
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return []RemoteInfo{}, nil
		}
		return nil, fmt.Errorf("list %q: %w", p, err)
	}
	result := make([]RemoteInfo, 0, len(infos))
	for _, info := range infos {
		childPath := path.Join(resolved, info.Name())
		result = append(result, RemoteInfo{
			Path:    childPath,
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime(),
		})
	}
	return result, nil
}

// MkdirAll creates a directory tree. It stats first so the common
// "already exists" case avoids relying on the server's 405 response, which
// is not a reliable existence signal across WebDAV implementations.
func (w *WebDAVFS) MkdirAll(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved := w.resolve(p)
	if _, err := w.client.Stat(resolved); err == nil {
		return nil // exists
	} else if !gowebdav.IsErrNotFound(err) {
		return fmt.Errorf("stat %q: %w", p, err)
	}
	if err := w.client.MkdirAll(resolved, 0o755); err != nil {
		// A concurrent create may have made the dir appear between Stat
		// and MkdirAll; re-stat to confirm before failing.
		if _, sErr := w.client.Stat(resolved); sErr == nil {
			return nil
		}
		return fmt.Errorf("mkdirall %q: %w", p, err)
	}
	return nil
}

// Close is a no-op for WebDAV (stateless HTTP).
func (w *WebDAVFS) Close() error { return nil }

// Ensure WebDAVFS satisfies RemoteFS at compile time.
var _ RemoteFS = (*WebDAVFS)(nil)
