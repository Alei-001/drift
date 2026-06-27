package sync

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"path"
	"strings"
	"time"

	"github.com/drift/drift/internal/config"
	"github.com/jlaffaye/ftp"
)

// FTPTransport implements the Transport interface over FTP/FTPS.
type FTPTransport struct {
	client   *ftp.ServerConn
	basePath string
}

// NewFTPTransport connects to an FTP server and returns a transport scoped
// to the project directory.
func NewFTPTransport(gcfg *config.GlobalConfig, remoteName string) (*FTPTransport, error) {
	addr := net.JoinHostPort(gcfg.Host, fmt.Sprintf("%d", EffectivePort(gcfg)))
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

	basePath := "/" + strings.Trim(gcfg.Path, "/")
	if basePath == "/" {
		basePath = "/" + remoteName
	} else {
		basePath = basePath + "/" + remoteName
	}

	return &FTPTransport{
		client:   client,
		basePath: basePath,
	}, nil
}

func (t *FTPTransport) absPath(key string) string {
	return path.Join(t.basePath, strings.TrimPrefix(key, "/"))
}

func (t *FTPTransport) Get(key string) (io.ReadCloser, error) {
	resp, err := t.client.Retr(t.absPath(key))
	if err != nil {
		return nil, fmt.Errorf("ftp get %s: %w", key, err)
	}
	return resp, nil
}

func (t *FTPTransport) Put(key string, data io.Reader) error {
	abs := t.absPath(key)
	parent := path.Dir(abs)
	if parent != "." && parent != "/" {
		if err := t.mkdirAll(parent); err != nil {
			return fmt.Errorf("mkdir parent: %w", err)
		}
	}
	return t.client.Stor(abs, data)
}

func (t *FTPTransport) Exists(key string) (bool, error) {
	abs := t.absPath(key)
	// Try SIZE as a quick existence check.
	if size, err := t.client.FileSize(abs); err == nil && size >= 0 {
		return true, nil
	}
	// Fallback: list parent directory.
	dir := path.Dir(abs)
	name := path.Base(abs)
	entries, err := t.client.List(dir)
	if err != nil {
		return false, nil
	}
	for _, e := range entries {
		if e.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (t *FTPTransport) GetRef(name string) (string, error) {
	return t.getTextFile(refKey(name))
}

func (t *FTPTransport) PutRef(name string, hash string) error {
	return t.putTextFile(refKey(name), hash)
}

func (t *FTPTransport) ListRefs() (map[string]string, error) {
	refs := make(map[string]string)
	refsDir := t.absPath("refs")
	if err := t.walkRefs(refsDir, "", refs); err != nil {
		return refs, nil
	}
	return refs, nil
}

func (t *FTPTransport) walkRefs(absDir, relDir string, refs map[string]string) error {
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
			if err := t.walkRefs(path.Join(absDir, e.Name), relPath, refs); err != nil {
				return err
			}
		} else if strings.HasSuffix(relPath, ".ref") {
			refName := strings.TrimSuffix(relPath, ".ref")
			resp, err := t.client.Retr(path.Join(absDir, e.Name))
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(resp)
			resp.Close()
			hash := strings.TrimSpace(string(data))
			if hash != "" {
				refs[refName] = hash
			}
		}
	}
	return nil
}

func (t *FTPTransport) mkdirAll(absDir string) error {
	parts := strings.Split(strings.TrimPrefix(absDir, "/"), "/")
	current := "/"
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		if err := t.client.MakeDir(current); err != nil && !isFTPAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (t *FTPTransport) getTextFile(key string) (string, error) {
	resp, err := t.client.Retr(t.absPath(key))
	if err != nil {
		return "", nil // not found = empty
	}
	defer resp.Close()
	data, err := io.ReadAll(resp)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (t *FTPTransport) putTextFile(key, content string) error {
	return t.Put(key, strings.NewReader(content+"\n"))
}

func (t *FTPTransport) Close() error {
	return t.client.Quit()
}

func isFTPAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "521") ||
		strings.Contains(s, "already exists") ||
		strings.Contains(s, "file exists")
}

var _ Transport = (*FTPTransport)(nil)
