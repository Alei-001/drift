package smb

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Alei-001/drift/internal/remote"
	smb2 "github.com/hirochachacha/go-smb2"
)

// init registers the smb protocol factory.
func init() {
	remote.Register("smb", NewSMBFS)
}

// SMBFS implements remote.RemoteFS over the SMB2/3 protocol using go-smb2.
// Unlike WebDAV (stateless HTTP), SMB holds a live TCP connection + session,
// so Close() must be called to release resources.
type SMBFS struct {
	conn    net.Conn
	session *smb2.Session
	share   *smb2.Share
	// subPath is the path within the share (the part of the URL after the
	// share name). All operations are relative to this path.
	subPath string
}

// NewSMBFS constructs an SMBFS from a remote.RemoteConfig. The URL format is:
//
//	smb://host[:port]/share[/path]
//
// The password is read from cfg.Options["_password"] (populated by
// resolveRemoteConfig from the user-level credentials.json).
// cfg.Options["domain"] optionally specifies the SMB domain.
func NewSMBFS(cfg remote.RemoteConfig) (remote.RemoteFS, error) {
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

// resolve joins a relative path with the SMB subPath. Per the remote.RemoteFS
// path contract, input paths have no leading slash; go-smb2 also requires
// relative paths (no leading "/"), so resolve keeps paths as-is, joining
// only the subPath prefix when configured. The root is represented by "."
// because go-smb2 rejects paths starting with "\" (which "/" maps to).
func (s *SMBFS) resolve(p string) string {
	if p == "" || p == "." {
		if s.subPath == "" || s.subPath == "/" {
			return "."
		}
		p = s.subPath
	} else {
		p = strings.ReplaceAll(p, "\\", "/")
		p = strings.TrimPrefix(p, "/")
		if s.subPath != "" && s.subPath != "/" {
			p = strings.TrimPrefix(s.subPath, "/") + "/" + p
		}
	}
	return path.Clean(p)
}

// Stat returns metadata for a remote path.
func (s *SMBFS) Stat(ctx context.Context, p string) (*remote.RemoteInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := s.share.Stat(s.resolve(p))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("stat %q: %w", p, os.ErrNotExist)
		}
		return nil, fmt.Errorf("stat %q: %w", p, err)
	}
	return &remote.RemoteInfo{
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
func (r *smbReadCloser) Close() error               { return r.file.Close() }

// Read opens a remote file for streaming. The caller must close the reader.
func (s *SMBFS) Read(ctx context.Context, p string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := s.share.Open(s.resolve(p))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("read %q: %w", p, os.ErrNotExist)
		}
		return nil, fmt.Errorf("read %q: %w", p, err)
	}
	return &smbReadCloser{file: f}, nil
}

// Write uploads a file atomically: it first writes to <path>.partial, then
// removes the target and renames the partial onto it. This prevents a
// network interruption from leaving a half-written object on the remote.
// go-smb2's Rename does not support overwrite, so the target is removed
// first (a missing target is not an error).
func (s *SMBFS) Write(ctx context.Context, p string, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := remote.EnsureRemoteDir(ctx, s, path.Dir(p)); err != nil {
		return err
	}
	resolved := s.resolve(p)
	partial := resolved + ".partial"
	f, err := s.share.Create(partial)
	if err != nil {
		return fmt.Errorf("create partial %q: %w", p, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		_ = s.share.Remove(partial)
		return fmt.Errorf("write partial data %q: %w", p, err)
	}
	if err := f.Close(); err != nil {
		_ = s.share.Remove(partial)
		return fmt.Errorf("close partial %q: %w", p, err)
	}
	// go-smb2 Rename does not overwrite; remove the target first. A missing
	// target is the common case and is silently ignored.
	_ = s.share.Remove(resolved)
	if err := s.share.Rename(partial, resolved); err != nil {
		_ = s.share.Remove(partial)
		return fmt.Errorf("rename partial to %q: %w", p, err)
	}
	return nil
}

// Remove deletes a remote file. A missing file is not an error.
func (s *SMBFS) Remove(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.share.Remove(s.resolve(p)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("remove %q: %w", p, err)
	}
	return nil
}

// List enumerates entries directly under a directory path (non-recursive).
func (s *SMBFS) List(ctx context.Context, p string) ([]remote.RemoteInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	basePath := s.resolve(p)
	infos, err := s.share.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []remote.RemoteInfo{}, nil
		}
		return nil, fmt.Errorf("list %q: %w", p, err)
	}
	result := make([]remote.RemoteInfo, 0, len(infos))
	for _, info := range infos {
		childPath := path.Join(basePath, info.Name())
		result = append(result, remote.RemoteInfo{
			Path:    childPath,
			Size:    info.Size(),
			IsDir:   info.IsDir(),
			ModTime: info.ModTime(),
		})
	}
	return result, nil
}

// MkdirAll creates a directory tree.
func (s *SMBFS) MkdirAll(ctx context.Context, p string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.share.MkdirAll(s.resolve(p), 0o755); err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("mkdirall %q: %w", p, err)
		}
	}
	return nil
}

// Close releases the SMB session and TCP connection. The share is unmounted
// and the session logged off in order; conn.Close is called last and its
// error is ignored because Logoff typically closes the underlying transport,
// making a subsequent conn.Close return a benign "closed" error.
func (s *SMBFS) Close() error {
	var errs []error
	if s.share != nil {
		if err := s.share.Umount(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.session != nil {
		if err := s.session.Logoff(); err != nil {
			errs = append(errs, err)
		}
	}
	// conn.Close after Logoff usually returns a "closed" error; ignore it.
	if s.conn != nil {
		_ = s.conn.Close()
	}
	if len(errs) > 0 {
		return fmt.Errorf("smb close: %w", errs[0])
	}
	return nil
}

// Ensure SMBFS satisfies remote.RemoteFS at compile time.
var _ remote.RemoteFS = (*SMBFS)(nil)
