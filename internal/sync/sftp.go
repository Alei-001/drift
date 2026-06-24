// sftp.go implements the SFTP transport for remote synchronization.
package sync

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPTransport implements the Transport interface over SFTP (SSH file transfer).
type SFTPTransport struct {
	sshClient  *ssh.Client
	sftpClient *sftp.Client
	basePath   string
}

// NewSFTPTransport connects to an SFTP server and returns a transport scoped
// to the project directory under the configured base path.
func NewSFTPTransport(gcfg *GlobalConfig, remoteName string) (*SFTPTransport, error) {
	addr := net.JoinHostPort(gcfg.Host, fmt.Sprintf("%d", gcfg.EffectivePort()))

	var authMethods []ssh.AuthMethod
	// Try key-based authentication first.
	if gcfg.KeyPath != "" {
		key, err := os.ReadFile(gcfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("sftp read key %s: %w", gcfg.KeyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("sftp parse key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	// Fall back to password authentication.
	if gcfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(gcfg.Password))
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("sftp: no authentication method (set key_path or password)")
	}

	config := &ssh.ClientConfig{
		User:            gcfg.Username,
		Auth:            authMethods,
		HostKeyCallback: knownHostsCallback(addr),
		Timeout:         30 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("sftp connect %s: %w", addr, err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("sftp init: %w", err)
	}

	// Build the base path.
	basePath := strings.Trim(gcfg.Path, "/")
	if basePath == "" {
		basePath = "/" + remoteName
	} else {
		basePath = "/" + basePath + "/" + remoteName
	}

	return &SFTPTransport{
		sshClient:  sshClient,
		sftpClient: sftpClient,
		basePath:   basePath,
	}, nil
}

func (t *SFTPTransport) absPath(remotePath string) string {
	clean := strings.TrimPrefix(remotePath, "/")
	return path.Join(t.basePath, clean)
}

func (t *SFTPTransport) Get(remotePath string, dst io.Writer) error {
	f, err := t.sftpClient.Open(t.absPath(remotePath))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return err
	}
	defer f.Close()
	_, err = io.Copy(dst, f)
	return err
}

func (t *SFTPTransport) Put(remotePath string, src io.Reader) error {
	abs := t.absPath(remotePath)
	// Ensure parent directory exists.
	parent := path.Dir(abs)
	if parent != "." && parent != "/" {
		if err := t.mkdirAll(parent); err != nil {
			return fmt.Errorf("sftp mkdir parent: %w", err)
		}
	}
	f, err := t.sftpClient.Create(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

func (t *SFTPTransport) Stat(remotePath string) (*RemoteStat, error) {
	info, err := t.sftpClient.Stat(t.absPath(remotePath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", os.ErrNotExist, remotePath)
		}
		return nil, err
	}
	return &RemoteStat{
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

func (t *SFTPTransport) List(prefix string) ([]string, error) {
	startPath := t.basePath
	if prefix != "" {
		startPath = path.Join(t.basePath, strings.TrimPrefix(prefix, "/"))
	}
	var files []string
	walker := t.sftpClient.Walk(startPath)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		if walker.Stat().IsDir() {
			continue
		}
		// Compute relative path by trimming the base path prefix.
		// Use HasPrefix with a trailing slash so that a base path of
		// "/foo" does not match "/foobar/x".
		full := walker.Path()
		prefix := t.basePath + "/"
		if !strings.HasPrefix(full, prefix) {
			continue
		}
		rel := strings.TrimPrefix(full, prefix)
		if rel == "" || rel == "." {
			continue
		}
		files = append(files, rel)
	}
	sort.Strings(files)
	return files, nil
}

func (t *SFTPTransport) Delete(remotePath string) error {
	err := t.sftpClient.Remove(t.absPath(remotePath))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func (t *SFTPTransport) Mkdir(remotePath string) error {
	return t.mkdirAll(t.absPath(remotePath))
}

// mkdirAll creates all directories in the path.
func (t *SFTPTransport) mkdirAll(absPath string) error {
	parts := strings.Split(strings.TrimPrefix(absPath, "/"), "/")
	current := "/"
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = path.Join(current, part)
		// Try to create; ignore error if it already exists.
		if err := t.sftpClient.Mkdir(current); err != nil && !os.IsExist(err) {
			// Check if it already exists as a directory.
			if info, statErr := t.sftpClient.Stat(current); statErr == nil && info.IsDir() {
				continue
			}
			return err
		}
	}
	return nil
}

func (t *SFTPTransport) Close() error {
	return errors.Join(t.sftpClient.Close(), t.sshClient.Close())
}

// knownHostsCallback returns an ssh.HostKeyCallback that verifies the
// server's host key against ~/.drift/known_hosts. On first connection to
// an unknown host, the key is automatically added (TOFU model, like SSH's
// StrictHostKeyChecking=accept-new).
func knownHostsCallback(addr string) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		hostsPath, err := knownHostsPath()
		if err != nil {
			return fmt.Errorf("cannot determine known_hosts path: %w", err)
		}

		// Extract host:port from addr for matching.
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
			port = "22"
		}

		keyLine := fmt.Sprintf("%s:%s %s %s\n", host, port, key.Type(), base64Key(key.Marshal()))

		// Check if the host key is already known.
		data, err := os.ReadFile(hostsPath)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.TrimSpace(line) == strings.TrimSpace(keyLine) {
					return nil // Key matches — verified.
				}
				if strings.HasPrefix(line, host+":") || strings.HasPrefix(line, host+" ") {
					// Host is known but key differs — reject (MITM protection).
					return fmt.Errorf("host key for %s changed (possible MITM attack); remove the old entry from %s if this is intentional", host, hostsPath)
				}
			}
		}

		// First connection — add the key to known_hosts (TOFU).
		if err := os.MkdirAll(filepath.Dir(hostsPath), 0755); err != nil {
			return fmt.Errorf("cannot create known_hosts directory: %w", err)
		}
		f, err := os.OpenFile(hostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("cannot open known_hosts: %w", err)
		}
		defer f.Close()
		if _, err := f.WriteString(keyLine); err != nil {
			return fmt.Errorf("cannot write to known_hosts: %w", err)
		}

		return nil
	}
}

// knownHostsPath returns the path to ~/.drift/known_hosts.
func knownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".drift", "known_hosts"), nil
}

// base64Key encodes a public key blob as base64 (standard encoding).
func base64Key(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
