// smb.go implements the SMB transport for remote synchronization.
package sync

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/drift/drift/internal/config"
	smb2 "github.com/hirochachacha/go-smb2"
)

// SMBTransport implements the Transport interface over SMB/CIFS.
type SMBTransport struct {
	session  *smb2.Session
	share    *smb2.Share
	basePath string // path within the share
}

// Compile-time guard to ensure SMBTransport implements Transport.
var _ Transport = (*SMBTransport)(nil)

// NewSMBTransport connects to an SMB share and returns a transport scoped
// to the project directory under the configured base path.
func NewSMBTransport(gcfg *config.GlobalConfig, remoteName string) (*SMBTransport, error) {
	addr := net.JoinHostPort(gcfg.Host, fmt.Sprintf("%d", EffectivePort(gcfg)))

	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("smb connect %s: %w", addr, err)
	}

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     gcfg.Username,
			Password: gcfg.Password,
		},
	}
	session, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("smb dial %s: %w", addr, err)
	}

	share, err := session.Mount(gcfg.Share)
	if err != nil {
		session.Logoff()
		return nil, fmt.Errorf("smb mount share %q: %w", gcfg.Share, err)
	}

	// Build the base path within the share.
	basePath := strings.Trim(gcfg.Path, "/")
	if basePath == "" {
		basePath = remoteName
	} else {
		basePath = basePath + "/" + remoteName
	}

	return &SMBTransport{
		session:  session,
		share:    share,
		basePath: basePath,
	}, nil
}

func (t *SMBTransport) absPath(remotePath string) string {
	clean := strings.TrimPrefix(remotePath, "/")
	return path.Join(t.basePath, clean)
}

func (t *SMBTransport) Get(remotePath string, dst io.Writer) error {
	f, err := t.share.Open(t.absPath(remotePath))
	if err != nil {
		if isSMBNotFound(err) {
			return fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return err
	}
	defer f.Close()
	_, err = io.Copy(dst, f)
	return err
}

func (t *SMBTransport) Put(remotePath string, src io.Reader) error {
	abs := t.absPath(remotePath)
	// Ensure parent directory exists.
	parent := path.Dir(abs)
	if parent != "." && parent != "/" {
		if err := t.mkdirAll(parent); err != nil {
			return fmt.Errorf("smb mkdir parent: %w", err)
		}
	}
	f, err := t.share.Create(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

func (t *SMBTransport) Stat(remotePath string) (*RemoteStat, error) {
	info, err := t.share.Stat(t.absPath(remotePath))
	if err != nil {
		if isSMBNotFound(err) {
			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return nil, err
	}
	return &RemoteStat{
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

func (t *SMBTransport) List(prefix string) ([]string, error) {
	startPath := t.basePath
	if prefix != "" {
		startPath = path.Join(t.basePath, strings.TrimPrefix(prefix, "/"))
	}
	var files []string
	err := t.walkSMB(startPath, "", &files)
	if err != nil {
		if isSMBNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// walkSMB recursively lists files under dirPath.
func (t *SMBTransport) walkSMB(absDir, relDir string, files *[]string) error {
	entries, err := t.share.ReadDir(absDir)
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
			if err := t.walkSMB(path.Join(absDir, name), relPath, files); err != nil {
				return err
			}
		} else {
			*files = append(*files, relPath)
		}
	}
	return nil
}

func (t *SMBTransport) Delete(remotePath string) error {
	err := t.share.Remove(t.absPath(remotePath))
	if err != nil && isSMBNotFound(err) {
		return nil
	}
	return err
}

func (t *SMBTransport) Mkdir(remotePath string) error {
	return t.mkdirAll(t.absPath(remotePath))
}

// mkdirAll creates all directories in the path.
func (t *SMBTransport) mkdirAll(absPath string) error {
	parts := strings.Split(strings.TrimPrefix(absPath, "/"), "/")
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		if err := t.share.Mkdir(current, 0755); err != nil && !isSMBAlreadyExists(err) {
			// Check if it already exists as a directory.
			if info, statErr := t.share.Stat(current); statErr == nil && info.IsDir() {
				continue
			}
			return err
		}
	}
	return nil
}

func (t *SMBTransport) Close() error {
	return errors.Join(t.share.Umount(), t.session.Logoff())
}

// isSMBNotFound checks if an SMB error indicates "file not found".
func isSMBNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "STATUS_OBJECT_NAME_NOT_FOUND") ||
		strings.Contains(s, "no such file") ||
		strings.Contains(s, "not found") ||
		os.IsNotExist(err)
}

// isSMBAlreadyExists checks if an SMB error indicates the directory already exists.
func isSMBAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "STATUS_OBJECT_NAME_COLLISION") ||
		strings.Contains(s, "already exists") ||
		os.IsExist(err)
}
