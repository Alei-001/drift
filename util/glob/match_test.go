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
