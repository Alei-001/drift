package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	driftsync "github.com/drift/drift/internal/sync"
)

func (a *App) Clone(remoteName, destDir string) (int, error) {
	gcfg, err := config.LoadGlobalConfig()
	if err != nil {
		return 0, err
	}
	if gcfg.Protocol == "" {
		return 0, fmt.Errorf("no remote configured (run 'drift remote setup' first)")
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
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, err
	}

	transport, err := driftsync.CreateTransport(gcfg, remoteName)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to remote: %w", err)
	}
	defer transport.Close()

	destStore := storage.NewStore(destDir)
	if err := destStore.Init(); err != nil {
		return 0, fmt.Errorf("init store: %w", err)
	}

	engine := driftsync.NewEngine(transport, destStore)
	if err := engine.Clone(); err != nil {
		return 0, fmt.Errorf("clone failed: %w", err)
	}

	mainHash := getRefByPrefix(destStore, "remotes/origin/heads/main")
	if mainHash == "" {
		mainHash = getRefByPrefix(destStore, "remotes/origin/main")
	}
	if mainHash == "" {
		return 0, fmt.Errorf("no main branch found on remote")
	}

	if err := destStore.SaveRef("main", mainHash); err != nil {
		return 0, fmt.Errorf("set main ref: %w", err)
	}
	if err := destStore.SaveRef("HEAD", "main"); err != nil {
		return 0, fmt.Errorf("set HEAD: %w", err)
	}

	commit, err := destStore.GetCommit(mainHash)
	if err != nil {
		return 0, fmt.Errorf("get commit: %w", err)
	}
	tree, err := destStore.GetTree(commit.TreeHash)
	if err != nil {
		return 0, err
	}

	reader := core.NewTreeReader(destStore)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return 0, err
	}

	for _, b := range blobs {
		if err := core.ValidateTreePath(b.Path); err != nil {
			return 0, fmt.Errorf("unsafe path from remote %q: %w", b.Path, err)
		}
		targetPath := filepath.Join(destDir, filepath.FromSlash(b.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return 0, err
		}
		if b.Mode == core.ModeSymlink {
			data, err := destStore.GetBlob(b.Hash)
			if err != nil {
				return 0, fmt.Errorf("read symlink %s: %w", b.Path, err)
			}
			if err := os.Symlink(string(data), targetPath); err != nil {
				return 0, err
			}
		} else {
			data, err := destStore.GetBlob(b.Hash)
			if err != nil {
				return 0, fmt.Errorf("read blob %s: %w", b.Path, err)
			}
			if err := os.WriteFile(targetPath, data, os.FileMode(core.ToOSFileMode(b.Mode))); err != nil {
				return 0, err
			}
		}
	}

	idx := &core.Index{}
	for _, b := range blobs {
		entry := core.IndexEntry{
			Path: b.Path,
			Hash: b.Hash,
			Mode: b.Mode,
		}
		if err := idx.Add(entry); err != nil {
			return 0, fmt.Errorf("add %s to index: %w", entry.Path, err)
		}
	}
	if err := destStore.SaveIndex(idx); err != nil {
		return 0, fmt.Errorf("save index: %w", err)
	}

	return len(blobs), nil
}

func getRefByPrefix(store localStore, prefix string) string {
	refs, err := store.ListRefs()
	if err != nil {
		return ""
	}
	for name, hash := range refs {
		if strings.HasPrefix(name, prefix) {
			return hash
		}
	}
	return ""
}

type localStore interface {
	GetRef(name string) (string, error)
	SaveRef(name string, hash string) error
	ListRefs() (map[string]string, error)
	GetCommit(hash string) (*core.Commit, error)
	GetTree(hash string) (*core.Tree, error)
}
