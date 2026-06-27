package sync

import (
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/drift/drift/internal/config"
	sftplib "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SFTPTransport implements the Transport interface over SFTP.
type SFTPTransport struct {
	client *sftplib.Client
	base   string
}

// NewSFTPTransport connects to an SFTP server and returns a transport scoped
// to the project directory.
func NewSFTPTransport(gcfg *config.GlobalConfig, remoteName string) (*SFTPTransport, error) {
	addr := net.JoinHostPort(gcfg.Host, fmt.Sprintf("%d", EffectivePort(gcfg)))

	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	authMethods := []ssh.AuthMethod{}
	if gcfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(gcfg.Password))
	}
	if gcfg.KeyPath != "" {
		keyPath := gcfg.KeyPath
		if strings.HasPrefix(keyPath, "~/") {
			if home != "" {
				keyPath = filepath.Join(home, keyPath[2:])
			}
		}
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read key %s: %w", keyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse key %s: %w", keyPath, err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	sshConfig := &ssh.ClientConfig{
		User:    gcfg.Username,
		Auth:    authMethods,
		Timeout: 30 * time.Second,
	}

	if gcfg.InsecureSkipVerify {
		sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		if home == "" {
			return nil, fmt.Errorf("cannot determine home directory for known_hosts; set insecure_skip_verify to true to bypass host key verification")
		}
		knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
		if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"host key verification required but %s not found\n"+
					"  Option 1: Create the known_hosts file (e.g. ssh-keyscan %s >> %s)\n"+
					"  Option 2: Set insecure_skip_verify to true in drift config",
				knownHostsPath, gcfg.Host, knownHostsPath)
		}
		cb, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read known_hosts %s: %w", knownHostsPath, err)
		}
		sshConfig.HostKeyCallback = cb
	}

	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh connect %s: %w", addr, err)
	}

	client, err := sftplib.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sftp init: %w", err)
	}

	base := strings.Trim(gcfg.Path, "/")
	if base == "" {
		base = remoteName
	} else {
		base = base + "/" + remoteName
	}

	return &SFTPTransport{
		client: client,
		base:   "/" + base,
	}, nil
}

func (t *SFTPTransport) absPath(key string) string {
	return path.Join(t.base, strings.TrimPrefix(key, "/"))
}

func (t *SFTPTransport) Get(key string) (io.ReadCloser, error) {
	f, err := t.client.Open(t.absPath(key))
	if err != nil {
		return nil, fmt.Errorf("sftp get %s: %w", key, err)
	}
	return f, nil
}

func (t *SFTPTransport) Put(key string, data io.Reader) error {
	abs := t.absPath(key)
	if err := t.client.MkdirAll(path.Dir(abs)); err != nil {
		return fmt.Errorf("mkdir parent: %w", err)
	}
	f, err := t.client.Create(abs)
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

func (t *SFTPTransport) Exists(key string) (bool, error) {
	_, err := t.client.Stat(t.absPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, nil
	}
	return true, nil
}

func (t *SFTPTransport) GetRef(name string) (string, error) {
	return t.getTextFile(refKey(name))
}

func (t *SFTPTransport) PutRef(name string, hash string) error {
	return t.putTextFile(refKey(name), hash)
}

func (t *SFTPTransport) ListRefs() (map[string]string, error) {
	refs := make(map[string]string)
	refsDir := t.absPath("refs")
	if err := t.walkRefs(refsDir, "", refs); err != nil {
		return refs, nil
	}
	return refs, nil
}

func (t *SFTPTransport) walkRefs(absDir, relDir string, refs map[string]string) error {
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
		} else if strings.HasSuffix(relPath, ".ref") {
			refName := strings.TrimSuffix(relPath, ".ref")
			f, err := t.client.Open(path.Join(absDir, name))
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

func (t *SFTPTransport) getTextFile(key string) (string, error) {
	f, err := t.client.Open(t.absPath(key))
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

func (t *SFTPTransport) putTextFile(key, content string) error {
	return t.Put(key, strings.NewReader(content+"\n"))
}

func (t *SFTPTransport) Close() error {
	return t.client.Close()
}

var _ Transport = (*SFTPTransport)(nil)
