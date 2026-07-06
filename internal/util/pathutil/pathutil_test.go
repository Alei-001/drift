package pathutil

import (
	"path/filepath"
	"testing"
)

func TestNormalize_ForwardSlashes(t *testing.T) {
	if got := Normalize("foo\\bar\\baz"); got != "foo/bar/baz" {
		t.Errorf("Normalize(%q) = %q, want %q", "foo\\bar\\baz", got, "foo/bar/baz")
	}
}

func TestNormalize_CleansPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"foo/./bar", "foo/bar"},
		{"foo/../bar", "bar"},
		{"./foo", "foo"},
		{"foo/", "foo"},
		{"", "."},
	}
	for _, tc := range tests {
		if got := Normalize(tc.input); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRelToWorkDir_RelativePath(t *testing.T) {
	got, err := RelToWorkDir("/project", "src/main.go")
	if err != nil {
		t.Fatalf("RelToWorkDir failed: %v", err)
	}
	if got != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", got)
	}
}

func TestRelToWorkDir_AbsolutePath(t *testing.T) {
	// Use t.TempDir() to obtain an OS-native absolute path so the test
	// works on both Unix (forward-slash roots) and Windows (drive-letter roots).
	workDir := t.TempDir()
	absPath := filepath.Join(workDir, "src", "main.go")
	got, err := RelToWorkDir(workDir, absPath)
	if err != nil {
		t.Fatalf("RelToWorkDir failed: %v", err)
	}
	if got != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", got)
	}
}

func TestRelToWorkDir_RejectsTraversal(t *testing.T) {
	tests := []string{
		"../etc/passwd",
		"../../etc/passwd",
		"..",
	}
	for _, input := range tests {
		_, err := RelToWorkDir("/project", input)
		if err == nil {
			t.Errorf("RelToWorkDir(%q) should have returned error for path traversal", input)
		}
	}
}

func TestRelToWorkDir_AllowsInnerTraversal(t *testing.T) {
	// foo/../bar should resolve to bar (stays within workspace)
	got, err := RelToWorkDir("/project", "foo/../bar")
	if err != nil {
		t.Fatalf("RelToWorkDir failed: %v", err)
	}
	if got != "bar" {
		t.Errorf("expected 'bar', got %q", got)
	}
}

func TestRel_ForwardSlashOutput(t *testing.T) {
	got, err := Rel("/project", "/project/src/main.go")
	if err != nil {
		t.Fatalf("Rel failed: %v", err)
	}
	if got != "src/main.go" {
		t.Errorf("expected 'src/main.go', got %q", got)
	}
}
