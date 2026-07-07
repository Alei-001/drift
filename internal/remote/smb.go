package remote

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	smb2 "github.com/hirochachacha/go-smb2"
)

// init registers the smb protocol factory.
func init() {
	Register("smb", NewSMBFS)
}

// SMBFS implements RemoteFS over the SMB2/3 protocol using go-smb2.
// Unlike WebDAV (stateless HTTP), SMB holds a live TCP connection + session,
// so Close() must be called to release resources.
type SMBFS struct {
	conn   net.Conn
	session *smb2.Session
	share  *smb2.Share
	// subPath is the path within the share (the part of the URL after the
	// share name). All operations are relative to this path.
	subPath string
}

// NewSMBFS constructs an SMBFS from a RemoteConfig. The URL format is:
//   smb://host[:port]/share[/path]
// The password is read from cfg.Options["_password"] (populated by
// resolveRemoteConfig from the user-level credentials.json).
// cfg.Options["domain"] optionally specifies the SMB domain.
func NewSMBFS(cfg RemoteConfig) (RemoteFS, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("smb: URL is required")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("smb: parse URL %q: %w", cfg.URL, err)
	}
	if u.Scheme != "smb" {
		return nil, fmt.Errorf("smb: URL scheme must be smb, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("smb: URL must have a host")
	}
	port := u.Port()
	if port == "" {
		port = "445"
	}

	// Parse path: /share[/subpath]
	urlPath := strings.TrimPrefix(u.Path, "/")
	if urlPath == "" {
		return nil, fmt.Errorf("smb: URL must include a share name (smb://host/share)")
	}
	parts := strings.SplitN(urlPath, "/", 2)
	shareName := parts[0]
	subPath := ""
	if len(parts) == 2 {
		subPath = "/" + parts[1]
	}

	password := cfg.Options["_password"]
	domain := cfg.Options["domain"]

	// Connect TCP.
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("smb: dial %s:%s: %w", host, port, err)
	}

	// SMB session.
	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     cfg.User,
			Password: password,
			Domain:   domain,
		},
	}
	session, err := d.Dial(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("smb: session: %w", err)
	}

	// Mount the share.
	share, err := session.Mount(shareName)
	if err != nil {
		session.Logoff()
		conn.Close()
		return nil, fmt.Errorf("smb: mount share %q: %w", shareName, err)
	}

	return &SMBFS{
		conn:    conn,
		session: session,
		share:   share,
		subPath: subPath,
	}, nil
}

// resolve joins a relative path with the SMB subPath. All paths use forward
// slashes; go-smb2 accepts them on all platforms.
func (s *SMBFS) resolve(p string) string {
	if p == "" || p == "." {
		p = "/"
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if s.subPath != "" && s.subPath != "/" {
		p = path.Join(s.subPath, p)
	}
	return path.Clean(p)
}

// Stat returns metadata for a remote path.
func (s *SMBFS) Stat(p string) (*RemoteInfo, error) {
	info, err := s.share.Stat(s.resolve(p))
	if err != nil {
		if os.IsNotExist(err) {
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

// readCloser wraps an *smb2.File to implement io.ReadCloser.
type smbReadCloser struct {
	file *smb2.File
}

func (r *smbReadCloser) Read(p []byte) (int, error) { return r.file.Read(p) }
func (r *smbReadCloser) Close() error                { return r.file.Close() }

// Read opens a remote file for streaming. The caller must close the reader.
func (s *SMBFS) Read(p string) (io.ReadCloser, error) {
	f, err := s.share.Open(s.resolve(p))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("read %q: %w", p, os.ErrNotExist)
		}
		return nil, fmt.Errorf("read %q: %w", p, err)
	}
	return &smbReadCloser{file: f}, nil
}

// Write uploads a file, creating parent directories as needed.
func (s *SMBFS) Write(p string, r io.Reader) error {
	resolved := s.resolve(p)
	// Ensure parent directory exists.
	parent := path.Dir(resolved)
	if parent != "/" && parent != "." {
		if err := s.share.MkdirAll(parent, 0o755); err != nil {
			// MkdirAll may return EEXIST; ignore that case.
			if !os.IsExist(err) {
				return fmt.Errorf("mkdir parent %q: %w", parent, err)
			}
		}
	}
	f, err := s.share.Create(resolved)
	if err != nil {
		return fmt.Errorf("create %q: %w", p, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write data %q: %w", p, err)
	}
	return nil
}

// Remove deletes a remote file. A missing file is not an error.
func (s *SMBFS) Remove(p string) error {
	if err := s.share.Remove(s.resolve(p)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", p, err)
	}
	return nil
}

// List enumerates entries directly under a directory path (non-recursive).
func (s *SMBFS) List(p string) ([]RemoteInfo, error) {
	infos, err := s.share.ReadDir(s.resolve(p))
	if err != nil {
		if os.IsNotExist(err) {
			return []RemoteInfo{}, nil
		}
		return nil, fmt.Errorf("list %q: %w", p, err)
	}
	result := make([]RemoteInfo, 0, len(infos))
	basePath := s.resolve(p)
	for _, info := range infos {
		childPath := path.Join(basePath, info.Name())
		result = append(result, RemoteInfo{
			Path:    childPath,
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime(),
		})
	}
	return result, nil
}

// MkdirAll creates a directory tree.
func (s *SMBFS) MkdirAll(p string) error {
	if err := s.share.MkdirAll(s.resolve(p), 0o755); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("mkdirall %q: %w", p, err)
		}
	}
	return nil
}

// Close releases the SMB session and TCP connection.
func (s *SMBFS) Close() error {
	var errs []error
	if s.share != nil {
		// Share.Umount is not exposed; session.Logoff closes everything.
	}
	if s.session != nil {
		if err := s.session.Logoff(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.conn != nil {
		if err := s.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("smb close: %v", errs[0])
	}
	return nil
}

// Ensure SMBFS satisfies RemoteFS at compile time.
var _ RemoteFS = (*SMBFS)(nil)
