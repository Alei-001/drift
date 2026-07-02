package glob

import "testing"

func TestMatch_SingleStar(t *testing.T) {
	t.Run("matches file in root", func(t *testing.T) {
		match, err := Match("*.tmp", "a.tmp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected *.tmp to match a.tmp")
		}
	})

	t.Run("does not match file in subdirectory", func(t *testing.T) {
		match, err := Match("*.tmp", "dir/a.tmp")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if match {
			t.Errorf("expected *.tmp to not match dir/a.tmp")
		}
	})
}

func TestMatch_DoubleStarSlash(t *testing.T) {
	t.Run("matches file in root", func(t *testing.T) {
		match, err := Match("**/*.psd", "a.psd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected **/*.psd to match a.psd")
		}
	})

	t.Run("matches file in one-level subdirectory", func(t *testing.T) {
		match, err := Match("**/*.psd", "dir/a.psd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected **/*.psd to match dir/a.psd")
		}
	})

	t.Run("matches file in nested subdirectory", func(t *testing.T) {
		match, err := Match("**/*.psd", "a/b/c.psd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected **/*.psd to match a/b/c.psd")
		}
	})
}

func TestMatch_PrefixDoubleStar(t *testing.T) {
	t.Run("matches direct child", func(t *testing.T) {
		match, err := Match("backup/**", "backup/x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected backup/** to match backup/x")
		}
	})

	t.Run("matches nested child", func(t *testing.T) {
		match, err := Match("backup/**", "backup/x/y")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected backup/** to match backup/x/y")
		}
	})

	t.Run("does not match directory itself", func(t *testing.T) {
		match, err := Match("backup/**", "backup")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if match {
			t.Errorf("expected backup/** to not match backup")
		}
	})
}

func TestMatch_DriftDir(t *testing.T) {
	t.Run("matches direct child of drift dir", func(t *testing.T) {
		match, err := Match("**/.drift/**", ".drift/x")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected **/.drift/** to match .drift/x")
		}
	})

	t.Run("matches nested drift dir", func(t *testing.T) {
		match, err := Match("**/.drift/**", "a/.drift/b")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected **/.drift/** to match a/.drift/b")
		}
	})
}

func TestMatch_QuestionMark(t *testing.T) {
	t.Run("matches single character", func(t *testing.T) {
		match, err := Match("?.txt", "a.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected ?.txt to match a.txt")
		}
	})

	t.Run("does not match multiple characters", func(t *testing.T) {
		match, err := Match("?.txt", "ab.txt")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if match {
			t.Errorf("expected ?.txt to not match ab.txt")
		}
	})
}

func TestMatch_ExactPath(t *testing.T) {
	t.Run("matches exact filename", func(t *testing.T) {
		match, err := Match("config.yml", "config.yml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected config.yml to match config.yml")
		}
	})

	t.Run("does not match in subdirectory", func(t *testing.T) {
		match, err := Match("config.yml", "x/config.yml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if match {
			t.Errorf("expected config.yml to not match x/config.yml")
		}
	})
}

func TestMatch_InvalidPattern(t *testing.T) {
	t.Run("unclosed bracket treated as literal", func(t *testing.T) {
		match, err := Match("abc[def", "abc[def")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match {
			t.Errorf("expected abc[def to match abc[def (literal bracket)")
		}
	})
}

// TestCompile_EquivalentToMatch verifies that Compile + Matcher.Match produces
// the same result as the convenience Match function for a range of patterns.
// This guards the backward-compatibility shim while exercising the precompiled
// path.
func TestCompile_EquivalentToMatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
	}{
		{"*.tmp", "a.tmp"},
		{"*.tmp", "dir/a.tmp"},
		{"**/*.psd", "a.psd"},
		{"**/*.psd", "dir/a.psd"},
		{"**/*.psd", "a/b/c.psd"},
		{"backup/**", "backup/x"},
		{"backup/**", "backup/x/y"},
		{"backup/**", "backup"},
		{"**/.drift/**", ".drift/x"},
		{"**/.drift/**", "a/.drift/b"},
		{"?.txt", "a.txt"},
		{"?.txt", "ab.txt"},
		{"config.yml", "config.yml"},
		{"config.yml", "x/config.yml"},
		{"/secret.txt", "secret.txt"},
		{"/secret.txt", "notes/secret.txt"},
	}
	for _, c := range cases {
		m, err := Compile(c.pattern)
		if err != nil {
			t.Fatalf("Compile(%q): %v", c.pattern, err)
		}
		gotPrecompiled := m.Match(c.path)
		gotConvenience, err := Match(c.pattern, c.path)
		if err != nil {
			t.Fatalf("Match(%q, %q): %v", c.pattern, c.path, err)
		}
		if gotPrecompiled != gotConvenience {
			t.Errorf("pattern=%q path=%q: precompiled=%v convenience=%v",
				c.pattern, c.path, gotPrecompiled, gotConvenience)
		}
		if m.Pattern() != c.pattern {
			t.Errorf("Pattern(): got %q want %q", m.Pattern(), c.pattern)
		}
	}
}

// TestCompile_ReusedAcrossManyPaths verifies that a single Matcher compiled
// once can be matched against many paths. This is the structural guarantee
// that drives the performance fix: readIgnorePatterns compiles each pattern
// once and isIgnored reuses the same Matcher across the entire walk.
func TestCompile_ReusedAcrossManyPaths(t *testing.T) {
	m, err := Compile("**/*.tmp")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for i := 0; i < 1000; i++ {
		// Root-level tmp files should match **/*.tmp.
		if !m.Match("a.tmp") {
			t.Fatalf("iteration %d: expected a.tmp to match", i)
		}
		// Nested tmp files should match.
		if !m.Match("dir/a.tmp") {
			t.Fatalf("iteration %d: expected dir/a.tmp to match", i)
		}
		// Non-tmp files should not match.
		if m.Match("a.txt") {
			t.Fatalf("iteration %d: expected a.txt to NOT match", i)
		}
	}
}

// BenchmarkMatch_Recompile measures the cost of the convenience Match path,
// which recompiles the regex on every call. This is the pre-fix baseline.
func BenchmarkMatch_Recompile(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Match("**/*.psd", "dir/a.psd"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMatch_Precompiled measures the cost of Matcher.Match when the
// pattern is compiled once and reused. The large gap versus
// BenchmarkMatch_Recompile demonstrates that no recompilation happens on the
// hot path.
func BenchmarkMatch_Precompiled(b *testing.B) {
	m, err := Compile("**/*.psd")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.Match("dir/a.psd")
	}
}
