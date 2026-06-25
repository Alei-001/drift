package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// changeTestEnv builds a working directory and a fake store for ComputeStatus tests.
type changeTestEnv struct {
	root        string
	store       *memoryStore
	commitTree  *Tree
	commitFiles map[string]string // path -> hash
}

func newChangeTestEnv(t *testing.T, files map[string]string) *changeTestEnv {
	t.Helper()
	root := t.TempDir()
	store := newMemoryStore()

	entries := make([]TreeEntry, 0, len(files))
	commitFiles := make(map[string]string, len(files))
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		hash := CalculateHash([]byte(content))
		store.blobs[hash] = []byte(content)
		entries = append(entries, TreeEntry{
			Name: filepath.Base(path),
			Type: BlobObject,
			Hash: hash,
			Mode: ModeRegular,
		})
		commitFiles[path] = hash
	}
	tree, err := NewTree(entries)
	if err != nil {
		t.Fatal(err)
	}
	store.trees[tree.Hash] = tree
	return &changeTestEnv{root: root, store: store, commitTree: tree, commitFiles: commitFiles}
}

// TestComputeStatus_CleanWorktree verifies that a worktree matching the commit reports clean status.
func TestComputeStatus_CleanWorktree(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	status, err := ComputeStatus(env.commitTree, &Index{}, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	if !status.IsClean() {
		t.Fatalf("expected clean status, got %+v", status)
	}
}

// TestComputeStatus_WorktreeModified verifies that modifying a tracked file reports Worktree=Modified.
func TestComputeStatus_WorktreeModified(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	if err := os.WriteFile(filepath.Join(env.root, "a.txt"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	status, err := ComputeStatus(env.commitTree, &Index{}, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["a.txt"]
	if !ok {
		t.Fatal("expected a.txt in status")
	}
	if fs.Worktree != Modified {
		t.Fatalf("expected Worktree=Modified, got %q", fs.Worktree)
	}
}

// TestComputeStatus_WorktreeDeleted verifies that deleting a tracked file reports Worktree=Deleted.
func TestComputeStatus_WorktreeDeleted(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	if err := os.Remove(filepath.Join(env.root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	status, err := ComputeStatus(env.commitTree, &Index{}, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["a.txt"]
	if !ok {
		t.Fatal("expected a.txt in status")
	}
	if fs.Worktree != Deleted {
		t.Fatalf("expected Worktree=Deleted, got %q", fs.Worktree)
	}
}

// TestComputeStatus_UntrackedFile verifies that a new file not in commit/index is Untracked.
func TestComputeStatus_UntrackedFile(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	if err := os.WriteFile(filepath.Join(env.root, "new.txt"), []byte("n"), 0644); err != nil {
		t.Fatal(err)
	}
	status, err := ComputeStatus(env.commitTree, &Index{}, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["new.txt"]
	if !ok {
		t.Fatal("expected new.txt in status")
	}
	if fs.Staging != Untracked || fs.Worktree != Untracked {
		t.Fatalf("expected Untracked/Untracked, got %q/%q", fs.Staging, fs.Worktree)
	}
}

// TestComputeStatus_StagedAdded verifies that staging a new file reports Staging=Added.
func TestComputeStatus_StagedAdded(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	if err := os.WriteFile(filepath.Join(env.root, "b.txt"), []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}
	idx := &Index{}
	info, _ := os.Lstat(filepath.Join(env.root, "b.txt"))
	idx.Add(IndexEntry{
		Path:       "b.txt",
		Hash:       CalculateHash([]byte("b")),
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       ModeRegular,
	})

	status, err := ComputeStatus(env.commitTree, idx, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["b.txt"]
	if !ok {
		t.Fatal("expected b.txt in status")
	}
	if fs.Staging != Added {
		t.Fatalf("expected Staging=Added, got %q", fs.Staging)
	}
}

// TestComputeStatus_StagedDeleted verifies that removing a tracked file from the index reports Staging=Deleted.
func TestComputeStatus_StagedDeleted(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	// Add an unrelated file to the index so hasStagedChanges becomes true;
	// a.txt is absent from the index, so it should be reported as Staging=Deleted.
	idx := &Index{}
	idx.Add(IndexEntry{
		Path:       "other.txt",
		Hash:       CalculateHash([]byte("other")),
		ModifiedAt: time.Now(),
		Size:       5,
		Mode:       ModeRegular,
	})

	status, err := ComputeStatus(env.commitTree, idx, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["a.txt"]
	if !ok {
		t.Fatal("expected a.txt in status")
	}
	if fs.Staging != Deleted {
		t.Fatalf("expected Staging=Deleted, got %q", fs.Staging)
	}
}

// TestComputeStatus_StagedDeleted_WorktreeUnmodified verifies the bug fix:
// when staging=Deleted and the worktree file matches the commit hash, Worktree must NOT be Modified.
func TestComputeStatus_StagedDeleted_WorktreeUnmodified(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	idx := &Index{}
	idx.Add(IndexEntry{
		Path:       "other.txt",
		Hash:       CalculateHash([]byte("other")),
		ModifiedAt: time.Now(),
		Size:       5,
		Mode:       ModeRegular,
	})

	status, err := ComputeStatus(env.commitTree, idx, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["a.txt"]
	if !ok {
		t.Fatal("expected a.txt in status")
	}
	if fs.Worktree == Modified {
		t.Fatalf("expected Worktree != Modified for unmodified worktree file, got Modified")
	}
}

// TestComputeStatus_StagedModified verifies that staging a modified file reports Staging=Modified.
func TestComputeStatus_StagedModified(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	idx := &Index{}
	info, _ := os.Lstat(filepath.Join(env.root, "a.txt"))
	idx.Add(IndexEntry{
		Path:       "a.txt",
		Hash:       CalculateHash([]byte("new content")),
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       ModeRegular,
	})

	status, err := ComputeStatus(env.commitTree, idx, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["a.txt"]
	if !ok {
		t.Fatal("expected a.txt in status")
	}
	if fs.Staging != Modified {
		t.Fatalf("expected Staging=Modified, got %q", fs.Staging)
	}
}

// TestComputeStatus_NilCommitTree verifies that a nil commit tree is handled gracefully.
func TestComputeStatus_NilCommitTree(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	idx := &Index{}
	info, _ := os.Lstat(filepath.Join(root, "a.txt"))
	idx.Add(IndexEntry{
		Path:       "a.txt",
		Hash:       CalculateHash([]byte("a")),
		ModifiedAt: info.ModTime(),
		Size:       info.Size(),
		Mode:       ModeRegular,
	})

	status, err := ComputeStatus(nil, idx, root, nil)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["a.txt"]
	if !ok {
		t.Fatal("expected a.txt in status")
	}
	if fs.Staging != Added {
		t.Fatalf("expected Staging=Added with nil commit tree, got %q", fs.Staging)
	}
}

// TestComputeStatus_StagedFileMissingFromWorktree verifies that a staged file missing on disk reports Worktree=Deleted.
func TestComputeStatus_StagedFileMissingFromWorktree(t *testing.T) {
	env := newChangeTestEnv(t, map[string]string{"a.txt": "a"})
	idx := &Index{}
	idx.Add(IndexEntry{
		Path:       "ghost.txt",
		Hash:       CalculateHash([]byte("ghost")),
		ModifiedAt: time.Now(),
		Size:       5,
		Mode:       ModeRegular,
	})

	status, err := ComputeStatus(env.commitTree, idx, env.root, env.store)
	if err != nil {
		t.Fatalf("ComputeStatus failed: %v", err)
	}
	fs, ok := status["ghost.txt"]
	if !ok {
		t.Fatal("expected ghost.txt in status")
	}
	if fs.Worktree != Deleted {
		t.Fatalf("expected Worktree=Deleted, got %q", fs.Worktree)
	}
}
