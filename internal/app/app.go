package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/drift/drift/internal/worktree"
)

type App struct {
	store  *storage.Store
	wt     *worktree.Worktree
	config *config.Config
	dir    string
}

func New(store *storage.Store, cfg *config.Config, dir string) *App {
	autoCRLF := ""
	if cfg != nil {
		autoCRLF = cfg.Core.AutoCRLF
	}
	return &App{
		store:  store,
		wt:     worktree.New(store, dir, autoCRLF),
		config: cfg,
		dir:    dir,
	}
}

func (a *App) Config() *config.Config {
	return a.config
}

func (a *App) Author() core.Signature {
	if a.config != nil && a.config.User.Name != "" {
		return core.Signature{
			Name:  a.config.User.Name,
			Email: a.config.User.Email,
		}
	}
	if gcfg, err := config.LoadGlobalConfig(); err == nil {
		return core.Signature{
			Name:  gcfg.User.Name,
			Email: gcfg.User.Email,
		}
	}
	return core.Signature{}
}

func (a *App) currentCommit() (*core.Commit, error) {
	branch := a.CurrentBranch()
	hash, err := a.store.GetRef(branch)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if hash == "" {
		return nil, nil
	}
	return a.findCommitByHash(hash)
}

func (a *App) findCommitByHash(hash string) (*core.Commit, error) {
	c, err := a.store.GetCommit(hash)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, fmt.Errorf("commit not found: %s", hash)
		}
		return nil, err
	}
	return c, nil
}

func (a *App) ResolveCommit(version string) (*core.Commit, error) {
	if hash := a.resolveTag(version); hash != "" {
		return a.findCommitByHash(hash)
	}

	if strings.Contains(version, "/") {
		parts := strings.SplitN(version, "/", 2)
		branchName, versionID := parts[0], parts[1]

		if _, err := a.store.GetRef(branchName); err != nil {
			return nil, fmt.Errorf("branch not found: %s", branchName)
		}

		commits, err := a.store.ListBranchCommits(branchName)
		if err != nil {
			return nil, err
		}
		for _, c := range commits {
			if c.Branch == branchName && matchCommitID(c, versionID) {
				return c, nil
			}
		}
		return nil, fmt.Errorf("version %s not found in branch %s", versionID, branchName)
	}

	if hash, err := a.store.GetRef(version); err == nil && hash != "" {
		if commit, err := a.findCommitByHash(hash); err == nil && commit != nil {
			return commit, nil
		}
	} else if err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to resolve %q as branch: %w", version, err)
	}

	currentBranch := a.CurrentBranch()
	currentCommits, err := a.store.ListBranchCommits(currentBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits for branch %s: %w", currentBranch, err)
	}
	for _, c := range currentCommits {
		if matchCommitID(c, version) {
			return c, nil
		}
	}

	refs, err := a.store.ListRefs()
	if err != nil {
		return nil, err
	}
	var found *core.Commit
	var foundBranch string
	for branchName := range refs {
		if branchName == "HEAD" || branchName == currentBranch {
			continue
		}
		if strings.HasPrefix(branchName, "tags/") {
			continue
		}
		commits, err := a.store.ListBranchCommits(branchName)
		if err != nil {
			return nil, fmt.Errorf("failed to list commits for branch %s: %w", branchName, err)
		}
		for _, c := range commits {
			if matchCommitID(c, version) {
				if found != nil {
					return nil, fmt.Errorf("ambiguous version %s: exists in both %s and %s branches. Use branch/ID format (e.g., %s/%s)",
						version, foundBranch, c.Branch, foundBranch, version)
				}
				found = c
				foundBranch = c.Branch
			}
		}
	}
	if found != nil {
		return found, nil
	}

	return nil, fmt.Errorf("version not found: %s", version)
}

func (a *App) resolveTag(version string) string {
	hash, err := a.store.GetRef("tags/" + version)
	if err != nil || hash == "" {
		return ""
	}
	return hash
}

func matchCommitID(c *core.Commit, version string) bool {
	return c.ID == version ||
		strings.HasPrefix(c.ID, version) ||
		strings.HasPrefix(c.Hash, version)
}

func (a *App) hasPendingStagedChanges(idx *core.Index, filters []string) (bool, error) {
	if len(idx.Entries) == 0 {
		return false, nil
	}

	commit, err := a.currentCommit()
	if err != nil {
		return false, err
	}
	if commit == nil {
		for _, entry := range idx.Entries {
			if worktree.PathMatchesAny(entry.Path, filters) {
				return true, nil
			}
		}
		return false, nil
	}

	tree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		return false, err
	}

	reader := core.NewTreeReader(a.store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return false, err
	}

	commitFiles := make(map[string]string, len(blobs))
	for _, b := range blobs {
		commitFiles[b.Path] = b.Hash
	}

	for _, entry := range idx.Entries {
		if !worktree.PathMatchesAny(entry.Path, filters) {
			continue
		}
		commitHash, exists := commitFiles[entry.Path]
		if !exists || commitHash != entry.Hash {
			return true, nil
		}
	}

	for path := range commitFiles {
		if !worktree.PathMatchesAny(path, filters) {
			continue
		}
		if _, err := idx.Entry(path); err != nil {
			return true, nil
		}
	}

	return false, nil
}
