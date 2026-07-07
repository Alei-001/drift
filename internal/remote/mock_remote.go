package remote

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// MockRemoteFS is an in-memory RemoteFS implementation for testing sync
// logic without a real protocol backend. It is safe for concurrent use.
type MockRemoteFS struct {
	mu    sync.Mutex
	files map[string]*mockFile
	dirs  map[string]bool
}

type mockFile struct {
	content []byte
	mtime   time.Time
}

// NewMockRemoteFS returns a fresh in-memory RemoteFS with an empty root.
func NewMockRemoteFS() *MockRemoteFS {
	return &MockRemoteFS{
		files: make(map[string]*mockFile),
		dirs:  map[string]bool{"/": true, ".": true},
	}
}

func (m *MockRemoteFS) normalize(p string) string {
	if p == "" {
		return "/"
	}
	// Use forward slashes for consistency, like real WebDAV/SMB paths.
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func (m *MockRemoteFS) parent(p string) string {
	p = m.normalize(p)
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/"
	}
	return p[:idx]
}

func (m *MockRemoteFS) ensureParents(p string) {
	d := m.parent(p)
	for d != "/" && d != "." {
		m.dirs[d] = true
		d = m.parent(d)
	}
}

// Stat returns metadata for a remote path.
func (m *MockRemoteFS) Stat(path string) (*RemoteInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.normalize(path)
	if f, ok := m.files[p]; ok {
		return &RemoteInfo{Path: p, Size: int64(len(f.content)), IsDir: false, ModTime: f.mtime}, nil
	}
	if m.dirs[p] {
		return &RemoteInfo{Path: p, Size: 0, IsDir: true, ModTime: time.Now()}, nil
	}
	return nil, fmt.Errorf("stat %q: %w", p, os.ErrNotExist)
}

// Read opens a remote file for reading.
func (m *MockRemoteFS) Read(path string) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.normalize(path)
	f, ok := m.files[p]
	if !ok {
		return nil, fmt.Errorf("read %q: %w", p, os.ErrNotExist)
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), f.content...))), nil
}

// Write uploads a file, creating parent directories as needed.
func (m *MockRemoteFS) Write(path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read upload: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.normalize(path)
	m.ensureParents(p)
	m.files[p] = &mockFile{content: data, mtime: time.Now()}
	return nil
}

// Remove deletes a remote file. A missing file is not an error.
func (m *MockRemoteFS) Remove(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.normalize(path)
	delete(m.files, p)
	return nil
}

// List enumerates entries directly under a directory path (non-recursive).
func (m *MockRemoteFS) List(path string) ([]RemoteInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.normalize(path)
	// Ensure trailing slash so prefix matching works.
	prefix := p
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	seen := make(map[string]bool)
	var result []RemoteInfo
	for fp, f := range m.files {
		if !strings.HasPrefix(fp, prefix) {
			continue
		}
		// Direct child only: no further slashes after the prefix.
		rest := fp[len(prefix):]
		if rest == "" || strings.Contains(rest, "/") {
			continue
		}
		if !seen[rest] {
			result = append(result, RemoteInfo{
				Path:    fp,
				Size:    int64(len(f.content)),
				IsDir:   false,
				ModTime: f.mtime,
			})
			seen[rest] = true
		}
	}
	for d := range m.dirs {
		if !strings.HasPrefix(d, prefix) {
			continue
		}
		rest := d[len(prefix):]
		if rest == "" || strings.Contains(rest, "/") {
			continue
		}
		if !seen[rest] {
			result = append(result, RemoteInfo{
				Path:  d,
				Size:  0,
				IsDir: true,
			})
			seen[rest] = true
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	if result == nil {
		result = []RemoteInfo{}
	}
	return result, nil
}

// MkdirAll creates a directory tree.
func (m *MockRemoteFS) MkdirAll(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.normalize(path)
	m.dirs[p] = true
	m.ensureParents(p)
	return nil
}

// Close is a no-op for the mock.
func (m *MockRemoteFS) Close() error { return nil }
