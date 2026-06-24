// ftp.go implements the FTP transport for remote synchronization.
package sync

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

// FTPTransport implements the Transport interface over FTP/FTPS.
type FTPTransport struct {
	client   *ftp.ServerConn
	basePath string // absolute base directory on the FTP server
}

// NewFTPTransport connects to an FTP server and returns a transport scoped
// to the project directory under the configured base path.
func NewFTPTransport(gcfg *GlobalConfig, remoteName string) (*FTPTransport, error) {
	addr := net.JoinHostPort(gcfg.Host, fmt.Sprintf("%d", gcfg.EffectivePort()))
	opts := []ftp.DialOption{ftp.DialWithTimeout(30 * time.Second)}
	if gcfg.TLS {
		tlsConfig := &tls.Config{
			ServerName:         gcfg.Host,
			InsecureSkipVerify: gcfg.InsecureSkipVerify,
		}
		opts = append(opts, ftp.DialWithTLS(tlsConfig))
	}

	client, err := ftp.Dial(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("ftp connect %s: %w", addr, err)
	}
	if err := client.Login(gcfg.Username, gcfg.Password); err != nil {
		client.Quit()
		return nil, fmt.Errorf("ftp login: %w", err)
	}

	// Build the base path: configured path + project name.
	basePath := strings.Trim(gcfg.Path, "/")
	if basePath == "" {
		basePath = "/" + remoteName
	} else {
		basePath = "/" + basePath + "/" + remoteName
	}

	return &FTPTransport{
		client:   client,
		basePath: basePath,
	}, nil
}

// absPath joins the base path with a relative remote path.
func (t *FTPTransport) absPath(remotePath string) string {
	clean := strings.TrimPrefix(remotePath, "/")
	return path.Join(t.basePath, clean)
}

func (t *FTPTransport) Get(remotePath string, dst io.Writer) error {
	resp, err := t.client.Retr(t.absPath(remotePath))
	if err != nil {
		if isFTPNotFound(err) {
			return fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return err
	}
	defer resp.Close()
	_, err = io.Copy(dst, resp)
	return err
}

func (t *FTPTransport) Put(remotePath string, src io.Reader) error {
	// Ensure parent directories exist.
	parent := path.Dir(remotePath)
	if parent != "." && parent != "/" {
		if err := t.mkdirAll(parent); err != nil {
			return fmt.Errorf("ftp mkdir parent: %w", err)
		}
	}
	return t.client.Stor(t.absPath(remotePath), src)
}

func (t *FTPTransport) Stat(remotePath string) (*RemoteStat, error) {
	// FTP doesn't have a direct stat; use LIST on the parent directory.
	dir := path.Dir(remotePath)
	name := path.Base(remotePath)
	entries, err := t.client.List(t.absPath(dir))
	if err != nil {
		if isFTPNotFound(err) {
			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return nil, err
	}
	for _, e := range entries {
		if e.Name == name {
			return &RemoteStat{
				Size:    int64(e.Size),
				ModTime: e.Time,
			}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
}

func (t *FTPTransport) List(prefix string) ([]string, error) {
	startPath := t.basePath
	if prefix != "" {
		startPath = path.Join(t.basePath, strings.TrimPrefix(prefix, "/"))
	}
	var files []string
	err := t.walkFTP(startPath, "", &files)
	if err != nil {
		if isFTPNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// walkFTP recursively lists files under dirPath, appending relative paths
// (relative to basePath) to files.
func (t *FTPTransport) walkFTP(absDir, relDir string, files *[]string) error {
	entries, err := t.client.List(absDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		relPath := path.Join(relDir, e.Name)
		if e.Type == ftp.EntryTypeFolder {
			if err := t.walkFTP(path.Join(absDir, e.Name), relPath, files); err != nil {
				return err
			}
		} else {
			*files = append(*files, relPath)
		}
	}
	return nil
}

func (t *FTPTransport) Delete(remotePath string) error {
	err := t.client.Delete(t.absPath(remotePath))
	if err != nil && isFTPNotFound(err) {
		return nil // already gone
	}
	return err
}

func (t *FTPTransport) Mkdir(remotePath string) error {
	return t.mkdirAll(remotePath)
}

// mkdirAll creates all directories in the path, similar to os.MkdirAll.
func (t *FTPTransport) mkdirAll(remotePath string) error {
	parts := strings.Split(strings.TrimPrefix(remotePath, "/"), "/")
	current := t.basePath
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		// MakeDir returns an error if the directory already exists on
		// some servers; ignore that error.
		if err := t.client.MakeDir(current); err != nil && !isFTPAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (t *FTPTransport) Close() error {
	return t.client.Quit()
}

// isFTPNotFound checks if an FTP error indicates "file not found".
// FTP 550 can mean both "not found" and "permission denied", so we check
// the message text to distinguish. This avoids treating permission errors
// as "not found" which could cause silent data loss in Delete.
func isFTPNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	// Only a 550 status with "no such file" or "not found" text is a
	// genuine not-found; other 550 errors (e.g. permission denied) are not.
	return strings.Contains(s, "550") &&
		(strings.Contains(s, "no such file") || strings.Contains(s, "not found"))
}

// isFTPAlreadyExists checks if an FTP error indicates the directory already exists.
func isFTPAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	// 521 is "Duplicate" (RFC 959), some servers use 550 with "exists" text.
	return strings.Contains(s, "521") ||
		strings.Contains(s, "already exists") ||
		strings.Contains(s, "file exists") ||
		errors.Is(err, os.ErrExist)
}
