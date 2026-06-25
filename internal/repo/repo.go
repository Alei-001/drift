// Package repo encapsulates repository business logic: saving versions,
// switching branches, restoring files, and managing names/history.
// It sits between the CLI/GUI layer and the storage/core/worktree layers.
package repo

import (
	"errors"
	"fmt"
	"strings"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/drift/drift/internal/worktree"
)

// Repository is the central business-logic type, combining store, worktree,
// and config. CLI commands and future GUI code both delegate to it.
type Repository struct {
	Store     *storage.Store
	WT        *worktree.Worktree
	Config    *config.Config
	Dir       string
	GlobalUser config.UserConfig // default author from global config
}

// New creates a Repository from the given store, config, and working directory.
func New(store *storage.Store, cfg *config.Config, dir string) *Repository {
	autoCRLF := ""
	if cfg != nil {
		autoCRLF = cfg.Core.AutoCRLF
	}
	return &Repository{
		Store:  store,
		WT:     worktree.New(store, dir, autoCRLF),
		Config: cfg,
		Dir:    dir,
	}
}

// Author resolves the commit author: project-level config takes precedence,
// falling back to the global user config. Returns empty strings if neither
// is set.
func (r *Repository) Author() core.Signature {
	if r.Config != nil && r.Config.User.Name != "" {
		return core.Signature{
			Name:  r.Config.User.Name,
			Email: r.Config.User.Email,
		}
	}
	return core.Signature{
		Name:  r.GlobalUser.Name,
		Email: r.GlobalUser.Email,
	}
}

// CurrentBranch returns the current branch from HEAD, defaulting to "main".
func (r *Repository) CurrentBranch() string {
	branch, err := r.Store.GetRef("HEAD")
	if err != nil || branch == "" {
		return "main"
	}
	return branch
}

// CurrentCommit returns the latest commit on the current branch, or nil
// if the branch has no commits yet.
func (r *Repository) CurrentCommit() (*core.Commit, error) {
	branch := r.CurrentBranch()
	hash, err := r.Store.GetRef(branch)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return r.FindCommitByHash(hash)
}

// FindCommitByHash loads a commit by its hash directly from the commit store.
func (r *Repository) FindCommitByHash(hash string) (*core.Commit, error) {
	c, err := r.Store.GetCommit(hash)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, fmt.Errorf("commit not found: %s", hash)
		}
		return nil, err
	}
	return c, nil
}

// ResolveCommit resolves a version specifier to a commit.
// Supported formats: name alias, branch name, commit ID (abbreviated hash),
// hash prefix, branch/ID (e.g. "main/a1b2c3d4").
func (r *Repository) ResolveCommit(version string) (*core.Commit, error) {
	// Check version name alias first.
	if hash := r.ResolveName(version); hash != "" {
		return r.FindCommitByHash(hash)
	}

	// branch/version format.
	if strings.Contains(version, "/") {
		parts := strings.SplitN(version, "/", 2)
		branchName := parts[0]
		versionID := parts[1]

		branchHash, err := r.Store.GetRef(branchName)
		if err != nil || branchHash == "" {
			return nil, fmt.Errorf("branch not found: %s", branchName)
		}

		commits, err := r.Store.ListBranchCommits(branchName)
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

	// Try branch name first (latest commit on that branch).
	if hash, err := r.Store.GetRef(version); err == nil && hash != "" {
		if commit, err := r.FindCommitByHash(hash); err == nil && commit != nil {
			return commit, nil
		}
	} else if err != nil && !errors.Is(err, storage.ErrObjectNotFound) {
		return nil, fmt.Errorf("failed to resolve %q as branch: %w", version, err)
	}

	// Try commit ID / hash prefix in current branch, then any branch.
	currentBranch := r.CurrentBranch()

	currentCommits, err := r.Store.ListBranchCommits(currentBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits for branch %s: %w", currentBranch, err)
	}
	for _, c := range currentCommits {
		if matchCommitID(c, version) {
			return c, nil
		}
	}

	// Search other branches for ambiguity.
	refs, err := r.Store.ListRefs()
	if err != nil {
		return nil, err
	}
	var found *core.Commit
	var foundBranch string
	for branchName := range refs {
		if branchName == "HEAD" || branchName == currentBranch {
			continue
		}
		// Skip version name aliases — they are not branches.
		if strings.HasPrefix(branchName, "names/") {
			continue
		}
		commits, err := r.Store.ListBranchCommits(branchName)
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

// matchCommitID checks whether a commit matches a version specifier.
// Matches by exact ID, ID prefix (abbreviated hash), or full hash prefix.
func matchCommitID(c *core.Commit, version string) bool {
	if c.ID == version {
		return true
	}
	if strings.HasPrefix(c.ID, version) {
		return true
	}
	if strings.HasPrefix(c.Hash, version) {
		return true
	}
	return false
}

// HasPendingStagedChanges checks whether the index has entries that differ
// from the current branch commit.
func (r *Repository) HasPendingStagedChanges(idx *core.Index, filters []string) (bool, error) {
	if len(idx.Entries) == 0 {
		return false, nil
	}

	commit, err := r.CurrentCommit()
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

	tree, err := r.Store.GetTree(commit.TreeHash)
	if err != nil {
		return false, err
	}

	reader := core.NewTreeReader(r.Store)
	blobs, err := reader.ListBlobs(tree, "")
	if err != nil {
		return false, err
	}

	commitFiles := make(map[string]string)
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
