// webdav.go implements the WebDAV transport for remote synchronization.
// This enables sync with WebDAV servers (Nextcloud, ownCloud, Synology NAS,
// 坚果云, etc.) without requiring a local mount.
package sync

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// WebDAVTransport implements the Transport interface over WebDAV.
type WebDAVTransport struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

// NewWebDAVTransport creates a transport for a WebDAV server.
// baseURL should be the full URL to the directory that will contain
// project subdirectories (e.g. https://cloud.example.com/remote.php/dav/files/user/drift).
func NewWebDAVTransport(baseURL, username, password string) *WebDAVTransport {
	return &WebDAVTransport{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// urlFor builds the full URL for a remote path.
func (t *WebDAVTransport) urlFor(remotePath string) string {
	// WebDAV paths use forward slashes.
	clean := strings.TrimPrefix(remotePath, "/")
	return t.baseURL + "/" + clean
}

// newRequest creates an authenticated HTTP request.
func (t *WebDAVTransport) newRequest(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if t.username != "" {
		req.SetBasicAuth(t.username, t.password)
	}
	return req, nil
}

// Get downloads a remote file and writes it to dst.
func (t *WebDAVTransport) Get(remotePath string, dst io.Writer) error {
	req, err := t.newRequest("GET", t.urlFor(remotePath), nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found: %s", remotePath)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", remotePath, resp.Status)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

// Put uploads src to the remote at remotePath.
func (t *WebDAVTransport) Put(remotePath string, src io.Reader) error {
	// Ensure parent directory exists (best effort).
	parent := path.Dir(remotePath)
	if parent != "." && parent != "/" {
		_ = t.Mkdir(parent)
	}

	req, err := t.newRequest("PUT", t.urlFor(remotePath), src)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PUT %s: %s", remotePath, resp.Status)
	}
	return nil
}

// Stat returns metadata about a remote file.
func (t *WebDAVTransport) Stat(remotePath string) (*RemoteStat, error) {
	// Use PROPFIND with Depth: 0 to get metadata without downloading.
	req, err := t.newRequest("PROPFIND", t.urlFor(remotePath), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "0")
	req.Header.Set("Content-Type", "application/xml")
	req.Body = io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:prop><D:getcontentlength/><D:getlastmodified/></D:prop></D:propfind>`))

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found: %s", remotePath)
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("PROPFIND %s: %s", remotePath, resp.Status)
	}

	var ms multistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, err
	}
	if len(ms.Responses) == 0 || len(ms.Responses[0].Propstat) == 0 {
		return nil, fmt.Errorf("empty PROPFIND response for %s", remotePath)
	}
	prop := ms.Responses[0].Propstat[0].Prop
	stat := &RemoteStat{}
	if prop.ContentLength != "" {
		fmt.Sscanf(prop.ContentLength, "%d", &stat.Size)
	}
	if prop.LastModified != "" {
		if t, err := http.ParseTime(prop.LastModified); err == nil {
			stat.ModTime = t
		}
	}
	return stat, nil
}

// List returns all files under the given prefix.
func (t *WebDAVTransport) List(prefix string) ([]string, error) {
	url := t.urlFor(prefix)
	req, err := t.newRequest("PROPFIND", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Depth", "infinity")
	req.Header.Set("Content-Type", "application/xml")
	req.Body = io.NopCloser(strings.NewReader(`<?xml version="1.0" encoding="utf-8"?>
<D:propfind xmlns:D="DAV:"><D:prop><D:resourcetype/></D:prop></D:propfind>`))

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("PROPFIND %s: %s", prefix, resp.Status)
	}

	var ms multistatus
	if err := xml.NewDecoder(resp.Body).Decode(&ms); err != nil {
		return nil, err
	}

	var files []string
	basePath := t.baseURL + "/"
	for _, r := range ms.Responses {
		// Skip directories (only want files).
		if len(r.Propstat) > 0 && r.Propstat[0].Prop.ResourceType.Collection {
			continue
		}
		// Extract the relative path from the href.
		href := strings.TrimPrefix(r.Href, basePath)
		href = strings.TrimPrefix(href, "/")
		// URL-decode the href (WebDAV servers return percent-encoded paths).
		href = path.Clean(href)
		if href == "." || href == "" {
			continue
		}
		// If a prefix was given, strip it from the result.
		if prefix != "" {
			href = strings.TrimPrefix(href, strings.TrimPrefix(prefix, "/"))
			href = strings.TrimPrefix(href, "/")
		}
		if href != "" {
			files = append(files, href)
		}
	}
	return files, nil
}

// Delete removes a file from the remote.
func (t *WebDAVTransport) Delete(remotePath string) error {
	req, err := t.newRequest("DELETE", t.urlFor(remotePath), nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil // already gone
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DELETE %s: %s", remotePath, resp.Status)
	}
	return nil
}

// Mkdir creates a directory on the remote (MKCOL).
func (t *WebDAVTransport) Mkdir(remotePath string) error {
	// Create parent directories first (best effort).
	parent := path.Dir(remotePath)
	if parent != "." && parent != "/" {
		_ = t.Mkdir(parent)
	}

	req, err := t.newRequest("MKCOL", t.urlFor(remotePath), nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// 201 Created or 405 Method Not Allowed (already exists) are OK.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusMethodNotAllowed {
		return fmt.Errorf("MKCOL %s: %s", remotePath, resp.Status)
	}
	return nil
}

// --- XML types for WebDAV PROPFIND responses ---

type multistatus struct {
	XMLName  xml.Name      `xml:"multistatus"`
	Responses []webdavResponse `xml:"response"`
}

type webdavResponse struct {
	Href     string         `xml:"href"`
	Propstat []webdavPropstat `xml:"propstat"`
}

type webdavPropstat struct {
	Prop webdavProp `xml:"prop"`
}

type webdavProp struct {
	ContentLength string       `xml:"getcontentlength,omitempty"`
	LastModified  string       `xml:"getlastmodified,omitempty"`
	ResourceType  webdavResType `xml:"resourcetype"`
}

type webdavResType struct {
	Collection bool `xml:"collection"`
}
