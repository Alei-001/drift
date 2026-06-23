// Package sync provides remote synchronization for drift projects.
//
// Phase 1 (current): local filesystem transport (NAS mount, synced folder).
// The remote is a plain directory containing project subdirectories, so
// users can browse files directly on the NAS.
//
// Future phases: WebDAV transport, automatic sync after save, conflict
// resolution.
package sync

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// GlobalConfig stores drift-wide settings that apply across all projects.
// It lives at ~/.drift/global.json so it survives project cloning.
type GlobalConfig struct {
	RemoteRoot string `json:"remote_root,omitempty"`
}

// globalConfigPathOverride allows tests to redirect the global config to a
// temp directory. When empty, the default ~/.drift/global.json is used.
var globalConfigPathOverride string

// globalConfigPath returns the path to ~/.drift/global.json.
// The home directory is resolved via os.UserHomeDir().
func globalConfigPath() (string, error) {
	if globalConfigPathOverride != "" {
		return globalConfigPathOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".drift", "global.json"), nil
}

// SetGlobalConfigPathForTest overrides the global config path. Pass an empty
// string to restore the default. Intended for testing only.
func SetGlobalConfigPathForTest(path string) {
	globalConfigPathOverride = path
}

// LoadGlobalConfig reads the global config, returning an empty config if
// the file does not exist yet.
func LoadGlobalConfig() (*GlobalConfig, error) {
	path, err := globalConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalConfig{}, nil
		}
		return nil, fmt.Errorf("cannot read global config: %w", err)
	}
	cfg := &GlobalConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("invalid global config: %w", err)
	}
	return cfg, nil
}

// SaveGlobalConfig writes the global config atomically.
func SaveGlobalConfig(cfg *GlobalConfig) error {
	path, err := globalConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ProjectSyncConfig is the per-project sync state stored in .drift/config.json
// under the "sync" key. It is managed by the cli package via config.Config.
type ProjectSyncConfig struct {
	Enabled   bool   `json:"enabled"`
	ProjectID string `json:"project_id,omitempty"`
	RemoteName string `json:"remote_name,omitempty"`
	LastSync  string `json:"last_sync,omitempty"`
}

// NewProjectID generates a random 16-byte hex project identifier.
// Called once at 'drift init' time.
func NewProjectID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback should never happen in practice; crypto/rand failure
		// is extremely rare on supported platforms.
		return fmt.Sprintf("fallback-%d", os.Getpid())
	}
	return hex.EncodeToString(b)
}

// LocalTransport implements sync over a local filesystem path (NAS mount,
// cloud-drive synced folder, USB drive, etc.).
type LocalTransport struct {
	remoteRoot string
}

// NewLocalTransport creates a transport for the given remote root directory.
func NewLocalTransport(remoteRoot string) *LocalTransport {
	return &LocalTransport{remoteRoot: remoteRoot}
}

// RemoteRoot returns the configured remote root path.
func (t *LocalTransport) RemoteRoot() string {
	return t.remoteRoot
}

// remoteProjectDir returns the path to a project on the remote.
func (t *LocalTransport) remoteProjectDir(remoteName string) string {
	return filepath.Join(t.remoteRoot, remoteName)
}

// ProjectExists reports whether a project with the given remote name exists
// on the remote.
func (t *LocalTransport) ProjectExists(remoteName string) (bool, error) {
	dir := t.remoteProjectDir(remoteName)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

// ListProjects returns the names of all projects on the remote.
func (t *LocalTransport) ListProjects() ([]string, error) {
	entries, err := os.ReadDir(t.remoteRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip hidden directories.
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// Only list directories that look like drift projects.
		if _, err := os.Stat(filepath.Join(t.remoteRoot, e.Name(), ".drift")); err == nil {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Clone copies an entire remote project to a local destination directory.
// The destination must not exist or must be empty. Both .drift/ and the
// working tree files are copied so the project is immediately usable.
func (t *LocalTransport) Clone(remoteName, destDir string) error {
	src := t.remoteProjectDir(remoteName)
	if exists, err := t.ProjectExists(remoteName); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("project %q not found on remote", remoteName)
	}

	// Destination must not exist or be empty.
	if info, err := os.Stat(destDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("destination %q exists and is not a directory", destDir)
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("destination %q is not empty", destDir)
		}
	}

	return copyDir(src, destDir)
}

// copyDir recursively copies src into dst. dst is created if it does not
// exist. Symlinks are skipped to avoid escaping the project root.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		// Skip symlinks for safety.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies a single file from src to dst, preserving mode.
func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
