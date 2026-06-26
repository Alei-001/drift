package sync

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

// WebDAVTransport implements the Transport interface over WebDAV.
type WebDAVTransport struct {
	client *gowebdav.Client
	base   string // base URL for the project
}

// NewWebDAVTransport creates a transport for a WebDAV server.
// baseURL is the full URL to the project directory on the server.
func NewWebDAVTransport(baseURL, username, password string, insecureSkipVerify bool) *WebDAVTransport {
	c := gowebdav.NewClient(baseURL, username, password)
	c.SetTimeout(60 * time.Second)
	if insecureSkipVerify {
		c.SetTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		})
	}
	return &WebDAVTransport{client: c, base: strings.TrimRight(baseURL, "/")}
}

func (t *WebDAVTransport) abs(key string) string {
	return t.base + "/" + strings.TrimPrefix(key, "/")
}

func (t *WebDAVTransport) Get(key string) (io.ReadCloser, error) {
	rc, err := t.client.ReadStream(t.abs(key))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return nil, fmt.Errorf("not found: %s", key)
		}
		return nil, err
	}
	return rc, nil
}

func (t *WebDAVTransport) Put(key string, data io.Reader) error {
	abs := t.abs(key)
	dir := path.Dir(abs)
	if dir != "." && dir != "/" {
		if err := t.client.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir parent: %w", err)
		}
	}
	return t.client.WriteStream(abs, data, 0644)
}

func (t *WebDAVTransport) Exists(key string) (bool, error) {
	_, err := t.client.Stat(t.abs(key))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (t *WebDAVTransport) GetRef(name string) (string, error) {
	return t.getTextFile(refKey(name))
}

func (t *WebDAVTransport) PutRef(name string, hash string) error {
	return t.putTextFile(refKey(name), hash)
}

func (t *WebDAVTransport) ListRefs() (map[string]string, error) {
	refs := make(map[string]string)
	refsDir := t.abs("refs")
	err := t.walkRefs(refsDir, "", refs)
	if err != nil {
		// No refs directory means no refs — that's fine for a new remote.
		return refs, nil
	}
	return refs, nil
}

func (t *WebDAVTransport) walkRefs(absDir, relDir string, refs map[string]string) error {
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
			if err := t.walkRefs(path.Join(absDir, name), relPath, refs); err != nil {
				return err
			}
		} else if refPath := relPath; strings.HasSuffix(refPath, ".ref") {
			refName := strings.TrimSuffix(refPath, ".ref")
			abs := path.Join(absDir, name)
			rc, err := t.client.ReadStream(abs)
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			hash := strings.TrimSpace(string(data))
			if hash != "" {
				refs[refName] = hash
			}
		}
	}
	return nil
}

func (t *WebDAVTransport) getTextFile(key string) (string, error) {
	rc, err := t.client.ReadStream(t.abs(key))
	if err != nil {
		if gowebdav.IsErrNotFound(err) {
			return "", nil
		}
		return "", err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (t *WebDAVTransport) putTextFile(key, content string) error {
	return t.Put(key, strings.NewReader(content+"\n"))
}

func (t *WebDAVTransport) Close() error { return nil }

// Compile-time guard.
var _ Transport = (*WebDAVTransport)(nil)
