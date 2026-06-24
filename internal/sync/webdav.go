// webdav.go implements the WebDAV transport for remote synchronization.
// This enables sync with WebDAV servers (Nextcloud, ownCloud, Synology NAS,
// 坚果云, etc.) without requiring a local mount.
//
// Implementation uses the mature github.com/studio-b12/gowebdav library
// instead of hand-rolled PROPFIND/PUT/GET XML handling.
package sync

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

// WebDAVTransport implements the Transport interface over WebDAV.
type WebDAVTransport struct {
	client   *gowebdav.Client
	basePath string // absolute base directory on the WebDAV server
}

// NewWebDAVTransport creates a transport for a WebDAV server.
// baseURL should be the full URL to the directory that will contain
// project subdirectories (e.g. https://cloud.example.com/remote.php/dav/files/user/drift).
func NewWebDAVTransport(baseURL, username, password string, insecureSkipVerify bool) *WebDAVTransport {
	c := gowebdav.NewClient(baseURL, username, password)
	c.SetTimeout(60 * time.Second)
	if insecureSkipVerify {
		c.SetTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		})
	}
	return &WebDAVTransport{
		client:   c,
		basePath: "/", // scope is already baked into baseURL by the caller
	}
}

// absPath joins the remote-relative path with the base path.
func (t *WebDAVTransport) absPath(remotePath string) string {
	clean := strings.TrimPrefix(remotePath, "/")
	if clean == "" {
		return t.basePath
	}
	return path.Join(t.basePath, clean)
}

// Get downloads a remote file and writes it to dst.
func (t *WebDAVTransport) Get(remotePath string, dst io.Writer) error {
	rc, err := t.client.ReadStream(t.absPath(remotePath))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return err
	}
	defer rc.Close()
	_, err = io.Copy(dst, rc)
	return err
}

// Put uploads src to the remote at remotePath.
func (t *WebDAVTransport) Put(remotePath string, src io.Reader) error {
	abs := t.absPath(remotePath)
	// Ensure parent directory exists.
	parent := path.Dir(abs)
	if parent != "." && parent != "/" {
		if err := t.client.MkdirAll(parent, 0755); err != nil {
			return fmt.Errorf("webdav mkdir parent: %w", err)
		}
	}
	return t.client.WriteStream(abs, src, 0644)
}

// Stat returns metadata about a remote file.
func (t *WebDAVTransport) Stat(remotePath string) (*RemoteStat, error) {
	info, err := t.client.Stat(t.absPath(remotePath))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return nil, err
	}
	return &RemoteStat{
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

// List returns all files under the given prefix.
func (t *WebDAVTransport) List(prefix string) ([]string, error) {
	startPath := t.basePath
	if prefix != "" {
		startPath = path.Join(t.basePath, strings.TrimPrefix(prefix, "/"))
	}
	var files []string
	if err := t.walkWebDAV(startPath, "", &files); err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// walkWebDAV recursively lists files under absDir, recording paths relative
// to the transport's basePath.
func (t *WebDAVTransport) walkWebDAV(absDir, relDir string, files *[]string) error {
	entries, err := t.client.ReadDir(absDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}
		relPath := path.Join(relDir, name)
		if e.IsDir() {
			if err := t.walkWebDAV(path.Join(absDir, name), relPath, files); err != nil {
				return err
			}
		} else {
			*files = append(*files, relPath)
		}
	}
	return nil
}

// Delete removes a file from the remote.
func (t *WebDAVTransport) Delete(remotePath string) error {
	err := t.client.Remove(t.absPath(remotePath))
	if err != nil && gowebdav.IsErrNotFound(err) {
		return nil // already gone
	}
	return err
}

// Mkdir creates a directory on the remote (MKCOL).
func (t *WebDAVTransport) Mkdir(remotePath string) error {
	return t.client.MkdirAll(t.absPath(remotePath), 0755)
}

// Close releases any resources held by the transport.
// gowebdav's Client is stateless (HTTP-based), so this is a no-op.
func (t *WebDAVTransport) Close() error { return nil }

// Compile-time guard to ensure WebDAVTransport implements Transport.
var _ Transport = (*WebDAVTransport)(nil)
