package core

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// buildWorktree creates a sample working directory for walker tests.
//
//	root/
//	├── a.txt
//	├── .drift/        (should be skipped)
//	├── .git/          (should be skipped)
//	├── sub/
//	│   ├── b.txt
//	│   └── c.log
//	└── .driftignore   (ignores *.log)
func buildWorktree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWrite := func(p, content string) {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("a.txt", "a")
	mustWrite("sub/b.txt", "b")
	mustWrite("sub/c.log", "c")
	mustWrite(".driftignore", "*.log\n")
	mustWrite(".drift/objects/.keep", "")
	mustWrite(".git/config", "")
	return root
}

// TestWalkWorkingDir_SkipsDriftAndGit verifies that .drift and .git directories are skipped.
func TestWalkWorkingDir_SkipsDriftAndGit(t *testing.T) {
	root := buildWorktree(t)
	var got []string
	err := WalkWorkingDir(root, func(path string, info os.FileInfo) error {
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkWorkingDir failed: %v", err)
	}
	sort.Strings(got)
	// .drift and .git directories are skipped; .driftignore is a regular file and is walked.
	// sub/c.log is ignored by .driftignore.
	want := []string{".driftignore", "a.txt", "sub/b.txt"}
	if len(got) != len(want) {
		t.Fatalf("expected %d files, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("file %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// TestWalkWorkingDir_EmptyDir verifies that walking an empty directory yields no files.
func TestWalkWorkingDir_EmptyDir(t *testing.T) {
	root := t.TempDir()
	var count int
	err := WalkWorkingDir(root, func(path string, info os.FileInfo) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("WalkWorkingDir failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 files, got %d", count)
	}
}

// TestWalkWorkingDirWithIgnore_SubdirectoryWalk verifies that walking a subdirectory
// still applies the project-root .driftignore patterns.
func TestWalkWorkingDirWithIgnore_SubdirectoryWalk(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on Windows due to abs-path normalization differences")
	}
	root := buildWorktree(t)
	subDir := filepath.Join(root, "sub")

	var got []string
	err := WalkWorkingDirWithIgnore(subDir, root, func(path string, info os.FileInfo) error {
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkWorkingDirWithIgnore failed: %v", err)
	}
	sort.Strings(got)
	// c.log should be ignored by the project-root .driftignore even when walking sub/.
	want := []string{"b.txt"}
	if len(got) != len(want) {
		t.Fatalf("expected %d files, got %d (%v)", len(want), len(got), got)
	}
	if got[0] != want[0] {
		t.Fatalf("got %q, want %q", got[0], want[0])
	}
}

// TestWalkWorkingDir_StopOnWalkError verifies that walker propagates errors from the callback.
func TestWalkWorkingDir_StopOnWalkError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	stopErr := errSentinel{}
	err := WalkWorkingDir(root, func(path string, info os.FileInfo) error {
		return stopErr
	})
	if err != stopErr {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "sentinel" }
