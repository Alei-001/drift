package core

import (
	"os"
	"testing"
)

// TestNormalizeMode_RegularFile verifies regular non-executable files map to ModeRegular.
func TestNormalizeMode_RegularFile(t *testing.T) {
	if got := NormalizeMode(0644); got != ModeRegular {
		t.Fatalf("NormalizeMode(0644) = %o, want %o", got, ModeRegular)
	}
}

// TestNormalizeMode_ExecutableFile verifies executable files map to ModeExecutable.
func TestNormalizeMode_ExecutableFile(t *testing.T) {
	if got := NormalizeMode(0755); got != ModeExecutable {
		t.Fatalf("NormalizeMode(0755) = %o, want %o", got, ModeExecutable)
	}
}

// TestNormalizeMode_Directory verifies directories map to ModeDir.
func TestNormalizeMode_Directory(t *testing.T) {
	if got := NormalizeMode(os.ModeDir | 0755); got != ModeDir {
		t.Fatalf("NormalizeMode(dir) = %o, want %o", got, ModeDir)
	}
}

// TestNormalizeMode_Symlink verifies symlinks map to ModeSymlink.
func TestNormalizeMode_Symlink(t *testing.T) {
	if got := NormalizeMode(os.ModeSymlink | 0777); got != ModeSymlink {
		t.Fatalf("NormalizeMode(symlink) = %o, want %o", got, ModeSymlink)
	}
}

// TestToOSFileMode_RoundTrip verifies ToOSFileMode returns expected modes for known constants.
func TestToOSFileMode_RoundTrip(t *testing.T) {
	cases := []struct {
		mode uint32
		want os.FileMode
	}{
		{ModeDir, os.ModeDir | 0755},
		{ModeExecutable, 0755},
		{ModeSymlink, os.ModeSymlink},
		{ModeRegular, 0644},
	}
	for _, c := range cases {
		got := ToOSFileMode(c.mode)
		if got != c.want {
			t.Fatalf("ToOSFileMode(%o) = %v, want %v", c.mode, got, c.want)
		}
	}
}
