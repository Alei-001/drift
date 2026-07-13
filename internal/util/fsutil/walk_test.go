package fsutil

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/util/glob"
)

func TestReadIgnoreFile(t *testing.T) {
	root := t.TempDir()
	ignoreBody := "# a comment\n\n*.tmp\n  **/*.psd  \n/secret.txt\n"
	ignorePath := filepath.Join(root, ".driftignore")
	if err := os.WriteFile(ignorePath, []byte(ignoreBody), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	patterns, err := ReadIgnoreFile(ignorePath)
	if err != nil {
		t.Fatalf("ReadIgnoreFile: %v", err)
	}

	want := []string{"*.tmp", "**/*.psd", "/secret.txt"}
	if len(patterns) != len(want) {
		t.Fatalf("got %d patterns, want %d: %v", len(patterns), len(want), patterns)
	}
	for i, w := range want {
		if patterns[i] != w {
			t.Errorf("patterns[%d] = %q, want %q", i, patterns[i], w)
		}
	}
}

func TestReadIgnoreFileWithBOM(t *testing.T) {
	root := t.TempDir()
	// Write file with UTF-8 BOM
	data := append([]byte{0xEF, 0xBB, 0xBF}, []byte("*.tmp\n#comment\n/secret.txt\n")...)
	ignorePath := filepath.Join(root, ".driftignore")
	if err := os.WriteFile(ignorePath, data, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	patterns, err := ReadIgnoreFile(ignorePath)
	if err != nil {
		t.Fatalf("ReadIgnoreFile: %v", err)
	}

	want := []string{"*.tmp", "/secret.txt"}
	if len(patterns) != len(want) {
		t.Fatalf("got %d patterns, want %d: %v", len(patterns), len(want), patterns)
	}
	for i, w := range want {
		if patterns[i] != w {
			t.Errorf("patterns[%d] = %q, want %q", i, patterns[i], w)
		}
	}
}

func TestReadIgnoreFileMissing(t *testing.T) {
	root := t.TempDir()
	patterns, err := ReadIgnoreFile(filepath.Join(root, ".driftignore"))
	if err != nil {
		t.Fatalf("ReadIgnoreFile: %v", err)
	}
	if patterns != nil {
		t.Fatalf("expected nil for missing file, got %v", patterns)
	}
}

// TestWalk_AnchoredIgnorePattern verifies that an anchored pattern such as
// "/secret.txt" only ignores "secret.txt" at the repository root and does
// not ignore "notes/secret.txt", whose basename would otherwise be matched
// by a naive basename quick-match.
func TestWalk_AnchoredIgnorePattern(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, ".driftignore"), []byte("/secret.txt\n"), 0644); err != nil {
		t.Fatalf("write .driftignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("top secret\n"), 0644); err != nil {
		t.Fatalf("write root secret.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "secret.txt"), []byte("not secret\n"), 0644); err != nil {
		t.Fatalf("write notes/secret.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "readme.md"), []byte("readme\n"), 0644); err != nil {
		t.Fatalf("write readme.md: %v", err)
	}

	var tracked []string
	err := Walk(root, ".driftignore", func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		tracked = append(tracked, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	sort.Strings(tracked)

	want := []string{".driftignore", "notes/secret.txt", "readme.md"}
	if len(tracked) != len(want) {
		t.Fatalf("tracked = %v, want %v", tracked, want)
	}
	for i := range want {
		if tracked[i] != want[i] {
			t.Fatalf("tracked = %v, want %v", tracked, want)
		}
	}
}

// TestReadIgnoreFileThenCompile verifies that ReadIgnoreFile returns the
// correct patterns and they can be compiled into reusable matchers.
func TestReadIgnoreFileThenCompile(t *testing.T) {
	root := t.TempDir()
	ignoreBody := strings.Join([]string{
		"# a comment",
		"",
		"*.tmp",
		"**/*.psd",
		"/secret.txt",
		"backup/**",
		"node_modules/",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ".driftignore"), []byte(ignoreBody), 0644); err != nil {
		t.Fatalf("write .driftignore: %v", err)
	}

	patterns, err := ReadIgnoreFile(filepath.Join(root, ".driftignore"))
	if err != nil {
		t.Fatalf("ReadIgnoreFile: %v", err)
	}

	wantPatterns := []string{"*.tmp", "**/*.psd", "/secret.txt", "backup/**", "node_modules/"}
	if len(patterns) != len(wantPatterns) {
		t.Fatalf("got %d patterns, want %d (one per non-comment pattern line)", len(patterns), len(wantPatterns))
	}
	for i, want := range wantPatterns {
		if patterns[i] != want {
			t.Errorf("patterns[%d] = %q, want %q", i, patterns[i], want)
		}
	}

	// Verify each pattern is valid and compilable.
	for _, p := range patterns {
		if _, err := glob.Compile(p); err != nil {
			t.Errorf("pattern %q failed to compile: %v", p, err)
		}
	}
}

// TestIsIgnored_PrecompiledLargeScale simulates the original performance bug
// scenario — 10,000 files visited against 20 patterns — and verifies that
// isIgnored produces correct results using only the precompiled rules.
//
// The rules slice is built once via compileIgnoreRules and reused across
// every file, so the regex compilation count is 20 (one per pattern) rather
// than 200,000 (20 patterns × 10,000 files × 2 calls). Because isIgnored only
// invokes Matcher.Match (never glob.Compile or glob.Match), no recompilation
// can occur on the hot path.
func TestIsIgnored_PrecompiledLargeScale(t *testing.T) {
	root := t.TempDir()
	patternLines := make([]string, 0, 20)
	for i := 0; i < 18; i++ {
		// Non-anchored basename patterns.
		patternLines = append(patternLines, "*.ext"+strconv.Itoa(i))
	}
	patternLines = append(patternLines, "**/*.psd")
	patternLines = append(patternLines, "/secret.txt")
	ignoreBody := strings.Join(patternLines, "\n")
	if err := os.WriteFile(filepath.Join(root, ".driftignore"), []byte(ignoreBody), 0644); err != nil {
		t.Fatalf("write .driftignore: %v", err)
	}

	rules := compileIgnoreRules(readPatternsOrFail(t, root, ".driftignore"))
	if len(rules) != 20 {
		t.Fatalf("expected 20 compiled rules, got %d", len(rules))
	}
	layers := []ignoreLayer{{relDir: "", rules: rules}}

	// Reuse the same layers slice across 10,000 files. isIgnored only
	// calls Matcher.Match here — no recompilation.
	ignoredCount := 0
	for i := 0; i < 10000; i++ {
		// Every 50th file matches *.psd and should be ignored.
		var rel string
		if i%50 == 0 {
			rel = "notes/file" + strconv.Itoa(i) + ".psd"
		} else {
			rel = "notes/file" + strconv.Itoa(i) + ".txt"
		}
		if isIgnored(rel, false, layers) {
			ignoredCount++
		}
	}
	wantIgnored := 10000 / 50
	if ignoredCount != wantIgnored {
		t.Fatalf("ignored %d files, want %d", ignoredCount, wantIgnored)
	}

	// Anchored pattern: /secret.txt matches only the root file.
	if !isIgnored("secret.txt", false, layers) {
		t.Errorf("expected /secret.txt to match secret.txt at root")
	}
	if isIgnored("notes/secret.txt", false, layers) {
		t.Errorf("expected /secret.txt to NOT match notes/secret.txt")
	}
}

// BenchmarkIsIgnored_Precompiled measures the per-file cost of isIgnored using
// the precompiled rules. Combined with BenchmarkMatch_Precompiled in the
// glob package this confirms the hot path performs no regex compilation.
func BenchmarkIsIgnored_Precompiled(b *testing.B) {
	patternLines := make([]string, 0, 20)
	for i := 0; i < 18; i++ {
		patternLines = append(patternLines, "*.ext"+strconv.Itoa(i))
	}
	patternLines = append(patternLines, "**/*.psd")
	patternLines = append(patternLines, "/secret.txt")

	rules := compileIgnoreRules(patternLines)
	layers := []ignoreLayer{{relDir: "", rules: rules}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isIgnored("notes/file.psd", false, layers)
	}
}

func readPatternsOrFail(t *testing.T, root, ignoreFile string) []string {
	t.Helper()
	patterns, err := ReadIgnoreFile(filepath.Join(root, ignoreFile))
	if err != nil {
		t.Fatalf("ReadIgnoreFile: %v", err)
	}
	return patterns
}

// walkAndCollect runs Walk on root and returns the sorted relative paths of
// all tracked files (non-directories).
func walkAndCollect(t *testing.T, root, ignoreFile string) []string {
	t.Helper()
	var tracked []string
	err := Walk(root, ignoreFile, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		tracked = append(tracked, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	sort.Strings(tracked)
	return tracked
}

// TestWalk_DirOnlyPattern verifies that a trailing "/" restricts a pattern
// to directories: the directory is skipped but a file with the same basename
// is still tracked.
func TestWalk_DirOnlyPattern(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".driftignore"), []byte("logs/\n"), 0644); err != nil {
		t.Fatalf("write .driftignore: %v", err)
	}
	// "logs" directory at root — should be ignored.
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "debug.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatalf("write logs/debug.txt: %v", err)
	}
	// "logs" file in a subdirectory — should NOT be ignored (dirOnly).
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "logs"), []byte("not a dir\n"), 0644); err != nil {
		t.Fatalf("write sub/logs: %v", err)
	}

	tracked := walkAndCollect(t, root, ".driftignore")
	want := []string{".driftignore", "sub/logs"}
	if len(tracked) != len(want) {
		t.Fatalf("tracked = %v, want %v", tracked, want)
	}
	for i := range want {
		if tracked[i] != want[i] {
			t.Errorf("tracked[%d] = %q, want %q", i, tracked[i], want[i])
		}
	}
}

// TestWalk_NegationPattern verifies that "!" un-ignores a previously matched
// file, following gitignore last-match-wins semantics.
func TestWalk_NegationPattern(t *testing.T) {
	root := t.TempDir()
	body := strings.Join([]string{"*.tmp", "!important.tmp"}, "\n")
	if err := os.WriteFile(filepath.Join(root, ".driftignore"), []byte(body), 0644); err != nil {
		t.Fatalf("write .driftignore: %v", err)
	}
	for _, name := range []string{"a.tmp", "important.tmp", "b.tmp"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	tracked := walkAndCollect(t, root, ".driftignore")
	want := []string{".driftignore", "important.tmp"}
	if len(tracked) != len(want) {
		t.Fatalf("tracked = %v, want %v", tracked, want)
	}
	for i := range want {
		if tracked[i] != want[i] {
			t.Errorf("tracked[%d] = %q, want %q", i, tracked[i], want[i])
		}
	}
}

// TestWalk_NestedIgnoreFile verifies that a .driftignore in a subdirectory
// can un-ignore files ignored by the root .driftignore.
func TestWalk_NestedIgnoreFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".driftignore"), []byte("*.txt\n"), 0644); err != nil {
		t.Fatalf("write .driftignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatalf("write top.txt: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	// Nested .driftignore un-ignores keep.txt but not drop.txt.
	nestedBody := strings.Join([]string{"!keep.txt"}, "\n")
	if err := os.WriteFile(filepath.Join(root, "sub", ".driftignore"), []byte(nestedBody), 0644); err != nil {
		t.Fatalf("write sub/.driftignore: %v", err)
	}
	for _, name := range []string{"keep.txt", "drop.txt"} {
		if err := os.WriteFile(filepath.Join(root, "sub", name), []byte("x\n"), 0644); err != nil {
			t.Fatalf("write sub/%s: %v", name, err)
		}
	}

	tracked := walkAndCollect(t, root, ".driftignore")
	want := []string{".driftignore", "sub/.driftignore", "sub/keep.txt"}
	if len(tracked) != len(want) {
		t.Fatalf("tracked = %v, want %v", tracked, want)
	}
	for i := range want {
		if tracked[i] != want[i] {
			t.Errorf("tracked[%d] = %q, want %q", i, tracked[i], want[i])
		}
	}
}
