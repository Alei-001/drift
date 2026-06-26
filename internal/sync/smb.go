package sync

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/drift/drift/internal/config"
	smb2 "github.com/hirochachacha/go-smb2"
)

// SMBTransport implements the Transport interface over SMB/CIFS.
type SMBTransport struct {
	session  *smb2.Session
	share    *smb2.Share
	basePath string
}

// NewSMBTransport connects to an SMB share and returns a transport scoped
// to the project directory.
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

func (t *SMBTransport) absPath(key string) string {
	return path.Join(t.basePath, strings.TrimPrefix(key, "/"))
}

func (t *SMBTransport) Get(key string) (io.ReadCloser, error) {
	f, err := t.share.Open(t.absPath(key))
	if err != nil {
		return nil, fmt.Errorf("smb get %s: %w", key, err)
	}
	return f, nil
}

func (t *SMBTransport) Put(key string, data io.Reader) error {
	abs := t.absPath(key)
	parent := path.Dir(abs)
	if parent != "." && parent != "/" {
		if err := t.mkdirAll(parent); err != nil {
			return fmt.Errorf("mkdir parent: %w", err)
		}
	}
	f, err := t.share.Create(abs)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, data)
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func (t *SMBTransport) Exists(key string) (bool, error) {
	_, err := t.share.Stat(t.absPath(key))
	if err != nil {
		if isSMBNotFound(err) {
			return false, nil
		}
		return false, nil
	}
	return true, nil
}

func (t *SMBTransport) GetRef(name string) (string, error) {
	return t.getTextFile(refKey(name))
}

func (t *SMBTransport) PutRef(name string, hash string) error {
	return t.putTextFile(refKey(name), hash)
}

func (t *SMBTransport) ListRefs() (map[string]string, error) {
	refs := make(map[string]string)
	refsDir := t.absPath("refs")
	if err := t.walkRefs(refsDir, "", refs); err != nil {
		return refs, nil
	}
	return refs, nil
}

func (t *SMBTransport) walkRefs(absDir, relDir string, refs map[string]string) error {
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
			if err := t.walkRefs(path.Join(absDir, name), relPath, refs); err != nil {
				return err
			}
		} else if strings.HasSuffix(relPath, ".ref") {
			refName := strings.TrimSuffix(relPath, ".ref")
			f, err := t.share.Open(path.Join(absDir, name))
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(f)
			f.Close()
			hash := strings.TrimSpace(string(data))
			if hash != "" {
				refs[refName] = hash
			}
		}
	}
	return nil
}

func (t *SMBTransport) mkdirAll(absDir string) error {
	parts := strings.Split(strings.TrimPrefix(absDir, "/"), "/")
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		if err := t.share.Mkdir(current, 0755); err != nil && !isSMBAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func (t *SMBTransport) getTextFile(key string) (string, error) {
	f, err := t.share.Open(t.absPath(key))
	if err != nil {
		return "", nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (t *SMBTransport) putTextFile(key, content string) error {
	return t.Put(key, strings.NewReader(content+"\n"))
}

func (t *SMBTransport) Close() error {
	return errors.Join(t.share.Umount(), t.session.Logoff())
}

func isSMBNotFound(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "STATUS_OBJECT_NAME_NOT_FOUND") ||
		strings.Contains(s, "not found") ||
		os.IsNotExist(err)
}

func isSMBAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "STATUS_OBJECT_NAME_COLLISION") ||
		strings.Contains(s, "already exists") ||
		os.IsExist(err)
}

var _ Transport = (*SMBTransport)(nil)
