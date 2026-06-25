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
	// "build" itself is a directory; dir-only patterns require IsIgnoredDir.
	if !m.IsIgnoredDir("build") {
		t.Fatal("expected build (dir) to be ignored")
	}
	// Contents are matched as files (the dir-only check applies only to the
	// final path component), so IsIgnored works for files inside build/.
	if !m.IsIgnored("build/out.txt") {
		t.Fatal("expected build/out.txt to be ignored")
	}
	if !m.IsIgnored("build/sub/x.txt") {
		t.Fatal("expected build/sub/x.txt to be ignored")
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

// TestIgnoreMatcher_WildmatchConformance exercises the ported wildmatch
// algorithm against patterns drawn from go-git's gitignore conformance suite.
// Each case is a (pattern, path, isDir, wantIgnored) tuple.
func TestIgnoreMatcher_WildmatchConformance(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		path    string
		isDir   bool
		want    bool
	}{
		// Bracket expressions and ranges.
		{"bracket range", "[a-c].txt", "b.txt", false, true},
		{"bracket range miss", "[a-c].txt", "d.txt", false, false},
		{"bracket negation", "[!a-c].txt", "d.txt", false, true},
		{"bracket negation miss", "[!a-c].txt", "a.txt", false, false},
		{"POSIX digit class", "[[:digit:]].log", "7.log", false, true},
		{"POSIX digit class miss", "[[:digit:]].log", "a.log", false, false},
		{"POSIX alpha class", "[[:alpha:]]/x", "m/x", false, true},
		// Escapes.
		{"escaped star", "\\*.txt", "*.txt", false, true},
		{"escaped star literal miss", "\\*.txt", "a.txt", false, false},
		{"escaped question", "\\?.txt", "?.txt", false, true},
		// ** in the middle.
		{"mid double-star", "src/**/test.go", "src/a/b/test.go", false, true},
		{"mid double-star direct", "src/**/test.go", "src/test.go", false, true},
		{"mid double-star miss", "src/**/test.go", "src/a/b/other.go", false, false},
		// Inclusion overrides exclusion.
		{"inclusion", "*.log\n!keep.log", "keep.log", false, false},
		{"inclusion then exclusion", "*.log\n!keep.log\nkeep.log", "keep.log", false, true},
		// Dir-only patterns.
		{"dir-only matches dir", "build/", "build", true, true},
		{"dir-only misses file", "build/", "build", false, false},
		{"dir-only matches contents", "build/", "build/x", false, true},
		// Leading slash anchors to root.
		{"anchored root", "/foo", "foo", false, true},
		{"anchored root subdir miss", "/foo", "a/foo", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".driftignore"), []byte(tc.pattern+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			m := LoadDriftIgnore(dir)
			var got bool
			if tc.isDir {
				got = m.IsIgnoredDir(tc.path)
			} else {
				got = m.IsIgnored(tc.path)
			}
			if got != tc.want {
				t.Fatalf("pattern %q path %q isDir=%v: got %v, want %v",
					tc.pattern, tc.path, tc.isDir, got, tc.want)
			}
		})
	}
}
