package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDriftIgnore_NoFile verifies that a missing .driftignore yields an empty matcher.
func TestLoadDriftIgnore_NoFile(t *testing.T) {
	m := LoadDriftIgnore(t.TempDir())
	if m.IsIgnored("anything") {
		t.Fatal("empty matcher should not ignore anything")
	}
}

// TestLoadDriftIgnore_CommentsAndBlanks verifies that comments and blank lines are skipped.
func TestLoadDriftIgnore_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	content := "# header\n\n  \n*.log\n"
	if err := os.WriteFile(filepath.Join(dir, ".driftignore"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m := LoadDriftIgnore(dir)
	if !m.IsIgnored("debug.log") {
		t.Fatal("expected debug.log to be ignored")
	}
	if m.IsIgnored("code.go") {
		t.Fatal("code.go should not be ignored")
	}
}

// TestIgnoreMatcher_ExactPath verifies exact path matching.
func TestIgnoreMatcher_ExactPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftignore"), []byte("secret/keys.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := LoadDriftIgnore(dir)
	if !m.IsIgnored("secret/keys.txt") {
		t.Fatal("expected secret/keys.txt to be ignored")
	}
	if m.IsIgnored("secret/other.txt") {
		t.Fatal("secret/other.txt should not be ignored")
	}
}

// TestIgnoreMatcher_DirectoryPattern verifies that a trailing slash matches the directory and its contents.
func TestIgnoreMatcher_DirectoryPattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftignore"), []byte("build/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := LoadDriftIgnore(dir)
	cases := []string{"build", "build/out.txt", "build/sub/x.txt"}
	for _, c := range cases {
		if !m.IsIgnored(c) {
			t.Fatalf("expected %q to be ignored", c)
		}
	}
	if m.IsIgnored("building.txt") {
		t.Fatal("building.txt should not be ignored")
	}
}

// TestIgnoreMatcher_WildcardStar verifies * glob matching on the basename.
func TestIgnoreMatcher_WildcardStar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftignore"), []byte("*.tmp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := LoadDriftIgnore(dir)
	if !m.IsIgnored("a.tmp") {
		t.Fatal("a.tmp should be ignored")
	}
	if !m.IsIgnored("dir/b.tmp") {
		t.Fatal("dir/b.tmp should be ignored (basename match)")
	}
	if m.IsIgnored("a.txt") {
		t.Fatal("a.txt should not be ignored")
	}
}

// TestIgnoreMatcher_DoubleStarMatchesEverything verifies that "**" matches any path.
func TestIgnoreMatcher_DoubleStarMatchesEverything(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftignore"), []byte("**\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := LoadDriftIgnore(dir)
	for _, p := range []string{"a", "a/b", "x/y/z.txt"} {
		if !m.IsIgnored(p) {
			t.Fatalf("expected %q to be ignored by **", p)
		}
	}
}

// TestIgnoreMatcher_DoubleStarSuffix verifies that "**/name" matches name at any depth.
func TestIgnoreMatcher_DoubleStarSuffix(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".driftignore"), []byte("**/node_modules\n"), 0644); err != nil {
		t.Fatal(err)
	}
	m := LoadDriftIgnore(dir)
	for _, p := range []string{"node_modules", "a/node_modules", "a/b/node_modules"} {
		if !m.IsIgnored(p) {
			t.Fatalf("expected %q to be ignored", p)
		}
	}
}
