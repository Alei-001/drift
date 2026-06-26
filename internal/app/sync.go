package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/drift/drift/internal/config"
	driftsync "github.com/drift/drift/internal/sync"
)

type SyncStatus struct {
	Enabled    bool
	RemoteName string
	LastSync   string
}

type SyncRemoteOptions struct {
	Host              string
	Port              int
	Path              string
	Username          string
	Password          string
	TLS               bool
	InsecureSkipVerify bool
	Share             string
	KeyPath           string
}

type SyncRemoteInfo struct {
	Protocol string
	Host     string
	Port     int
	Path     string
	Username string
	TLS      bool
	Share    string
}

func (a *App) SyncEnable() error {
	if !a.IsInitialized() {
		return fmt.Errorf("not a drift repository")
	}

	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	if gcfg.Protocol == "" {
		return fmt.Errorf("no remote configured (run 'drift sync remote --protocol <...>' first)")
	}

	remoteName := filepath.Base(a.dir)

	if a.config.Sync.ProjectID == "" {
		a.config.Sync.ProjectID = config.NewProjectID()
	}
	a.config.Sync.Enabled = true
	a.config.Sync.RemoteName = remoteName

	if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if gcfg.Protocol == "local" {
		remoteDir := filepath.Join(gcfg.Path, remoteName)
		if err := os.MkdirAll(remoteDir, 0755); err != nil {
			return fmt.Errorf("failed to create remote project dir: %w", err)
		}
	}

	return nil
}

func (a *App) SyncDisable() error {
	if !a.IsInitialized() {
		return fmt.Errorf("not a drift repository")
	}

	a.config.Sync.Enabled = false
	if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	return nil
}

type SyncStats struct {
	Pushed        int
	Pulled        int
	RemoteDeleted int
	LocalDeleted  int
	Conflicts     int
}

func (a *App) SyncNow() (*SyncStats, error) {
	if !a.IsInitialized() {
		return nil, fmt.Errorf("not a drift repository")
	}

	if !a.config.Sync.Enabled {
		return nil, fmt.Errorf("sync is not enabled (run 'drift sync enable')")
	}

	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}
	if gcfg.Protocol == "" {
		return nil, fmt.Errorf("no remote configured")
	}

	remoteName := a.config.Sync.RemoteName

	if gcfg.Protocol == "local" {
		remoteDir := filepath.Join(gcfg.Path, remoteName)
		if err := os.MkdirAll(remoteDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to access remote: %w", err)
		}
	}

	transport, err := driftsync.ProjectTransportForConfig(gcfg, remoteName)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to remote: %w", err)
	}
	defer transport.Close()

	engine := driftsync.NewEngine(transport, a.config.Sync.ProjectID)
	result, err := engine.Sync(a.dir)
	if err != nil {
		return nil, err
	}

	a.config.Sync.LastSync = time.Now().Format(time.RFC3339)
	if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
		return nil, err
	}

	return &SyncStats{
		Pushed:        len(result.Pushed),
		Pulled:        len(result.Pulled),
		RemoteDeleted: len(result.RemoteDeleted),
		LocalDeleted:  len(result.LocalDeleted),
		Conflicts:     len(result.Conflicts),
	}, nil
}

func (a *App) SyncStatus() (*SyncStatus, error) {
	if !a.IsInitialized() {
		return nil, fmt.Errorf("not a drift repository")
	}

	return &SyncStatus{
		Enabled:    a.config.Sync.Enabled,
		RemoteName: a.config.Sync.RemoteName,
		LastSync:   a.config.Sync.LastSync,
	}, nil
}

func (a *App) SyncEnabled() bool {
	return a.IsInitialized() && a.config.Sync.Enabled
}

func (a *App) AutoSync() error {
	if !a.SyncEnabled() {
		return nil
	}
	_, err := a.SyncNow()
	return err
}

func (a *App) SyncRemoteSet(protocol string, opts SyncRemoteOptions) error {
	gcfg := &config.GlobalConfig{
		Protocol:           protocol,
		Host:               opts.Host,
		Port:               opts.Port,
		Path:               opts.Path,
		Username:           opts.Username,
		Password:           opts.Password,
		TLS:                opts.TLS,
		InsecureSkipVerify: opts.InsecureSkipVerify,
		Share:              opts.Share,
		KeyPath:            opts.KeyPath,
	}

	if protocol == "local" {
		abs, err := filepath.Abs(opts.Path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("cannot access %q: %w", abs, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%q is not a directory", abs)
		}
		gcfg.Path = abs
	}

	return config.SaveGlobalConfig(gcfg)
}

func (a *App) SyncRemoteShow() (*SyncRemoteInfo, error) {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}
	if gcfg.Protocol == "" {
		return nil, fmt.Errorf("no remote configured")
	}
	return &SyncRemoteInfo{
		Protocol: gcfg.Protocol,
		Host:     gcfg.Host,
		Port:     gcfg.Port,
		Path:     gcfg.Path,
		Username: gcfg.Username,
		TLS:      gcfg.TLS,
		Share:    gcfg.Share,
	}, nil
}

func (a *App) SyncRemoteUnset() error {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	gcfg.Protocol = ""
	gcfg.Host = ""
	gcfg.Port = 0
	gcfg.Path = ""
	gcfg.Username = ""
	gcfg.Password = ""
	gcfg.TLS = false
	gcfg.InsecureSkipVerify = false
	gcfg.Share = ""
	gcfg.KeyPath = ""
	if err := config.SaveGlobalConfig(gcfg); err != nil {
		return err
	}

	// Also disable sync in local config to avoid "no remote configured"
	// errors on subsequent sync operations.
	if a.IsInitialized() && a.config.Sync.Enabled {
		a.config.Sync.Enabled = false
		if err := config.SaveConfig(a.store.DriftDir(), a.config); err != nil {
			return fmt.Errorf("failed to disable sync: %w", err)
		}
	}

	return nil
}

func (a *App) Clone(remoteName, destDir string) (int, error) {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return 0, err
	}
	if gcfg.Protocol == "" {
		return 0, fmt.Errorf("no remote configured (run 'drift sync remote --protocol <...>' first)")
	}

	if !filepath.IsAbs(destDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return 0, err
		}
		destDir = filepath.Join(cwd, destDir)
	}

	if info, err := os.Stat(destDir); err == nil {
		if !info.IsDir() {
			return 0, fmt.Errorf("destination %q exists and is not a directory", destDir)
		}
		entries, err := os.ReadDir(destDir)
		if err != nil {
			return 0, err
		}
		if len(entries) > 0 {
			return 0, fmt.Errorf("destination %q is not empty", destDir)
		}
	}

	if gcfg.Protocol == "local" {
		transport := driftsync.NewLocalTransport(gcfg.Path)
		defer transport.Close()
		exists, err := transport.ProjectExists(remoteName)
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, fmt.Errorf("project %q not found on remote %s", remoteName, gcfg.Path)
		}
		if err := transport.Clone(remoteName, destDir); err != nil {
			return 0, fmt.Errorf("clone failed: %w", err)
		}
		// Count files in the cloned directory.
		count := 0
		filepath.Walk(destDir, func(_ string, info os.FileInfo, _ error) error {
			if info != nil && !info.IsDir() {
				count++
			}
			return nil
		})
		return count, nil
	}

	transport, err := driftsync.ProjectTransportForConfig(gcfg, remoteName)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to remote: %w", err)
	}
	defer transport.Close()

	files, err := transport.List("")
	if err != nil {
		return 0, fmt.Errorf("failed to list remote files: %w", err)
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("project %q not found or empty on remote", remoteName)
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, err
	}

	var downloaded int
	for _, remotePath := range files {
		cleaned := filepath.Clean(filepath.FromSlash(remotePath))
		if filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "..") {
			return downloaded, fmt.Errorf("invalid remote path: %s", remotePath)
		}
		localPath := filepath.Join(destDir, filepath.FromSlash(remotePath))
		if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
			return downloaded, err
		}
		f, err := os.Create(localPath)
		if err != nil {
			return downloaded, err
		}
		if err := transport.Get(remotePath, f); err != nil {
			f.Close()
			return downloaded, fmt.Errorf("failed to download %s: %w", remotePath, err)
		}
		f.Close()
		downloaded++
	}

	return downloaded, nil
}
