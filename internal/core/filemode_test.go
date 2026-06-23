package core

import (
	"os"
	"testing"
)

// TestNormalizeMode_RegularFile verifies regular non-executable files map to ModeRegular.
func TestNormalizeMode_RegularFile(t *testing.T) {
	got, err := NormalizeMode(0o644)
	if err != nil {
		t.Fatalf("NormalizeMode(0644) error: %v", err)
	}
	if got != ModeRegular {
		t.Fatalf("NormalizeMode(0644) = %o, want %o", got, ModeRegular)
	}
}

// TestNormalizeMode_ExecutableFile verifies executable files map to ModeExecutable.
func TestNormalizeMode_ExecutableFile(t *testing.T) {
	got, err := NormalizeMode(0o755)
	if err != nil {
		t.Fatalf("NormalizeMode(0755) error: %v", err)
	}
	if got != ModeExecutable {
		t.Fatalf("NormalizeMode(0755) = %o, want %o", got, ModeExecutable)
	}
}

// TestNormalizeMode_Directory verifies directories map to ModeDir.
func TestNormalizeMode_Directory(t *testing.T) {
	got, err := NormalizeMode(os.ModeDir | 0o755)
	if err != nil {
		t.Fatalf("NormalizeMode(dir) error: %v", err)
	}
	if got != ModeDir {
		t.Fatalf("NormalizeMode(dir) = %o, want %o", got, ModeDir)
	}
}

// TestNormalizeMode_Symlink verifies symlinks map to ModeSymlink.
func TestNormalizeMode_Symlink(t *testing.T) {
	got, err := NormalizeMode(os.ModeSymlink | 0o777)
	if err != nil {
		t.Fatalf("NormalizeMode(symlink) error: %v", err)
	}
	if got != ModeSymlink {
		t.Fatalf("NormalizeMode(symlink) = %o, want %o", got, ModeSymlink)
	}
}

// TestNormalizeMode_Unsupported verifies unsupported types return an error.
func TestNormalizeMode_Unsupported(t *testing.T) {
	_, err := NormalizeMode(os.ModeNamedPipe | 0o644)
	if err == nil {
		t.Fatalf("NormalizeMode(pipe) expected error, got nil")
	}
}

// TestToOSFileMode_RoundTrip verifies ToOSFileMode returns expected modes for known constants.
func TestToOSFileMode_RoundTrip(t *testing.T) {
	cases := []struct {
		mode uint32
		want os.FileMode
	}{
		{ModeDir, os.ModeDir | 0o755},
		{ModeExecutable, 0o755},
		{ModeSymlink, os.ModeSymlink | 0o777},
		{ModeRegular, 0o644},
	}
	for _, c := range cases {
		got := ToOSFileMode(c.mode)
		if got != c.want {
			t.Fatalf("ToOSFileMode(%o) = %v, want %v", c.mode, got, c.want)
		}
	}
}

// TestIsMalformed verifies malformed mode detection.
func TestIsMalformed(t *testing.T) {
	if IsMalformed(ModeRegular) {
		t.Fatalf("ModeRegular should not be malformed")
	}
	if IsMalformed(ModeSymlink) {
		t.Fatalf("ModeSymlink should not be malformed")
	}
	if !IsMalformed(0o170000) {
		t.Fatalf("0o170000 should be malformed")
	}
}

// TestNormalizeModeForPath_RegularFile verifies that a regular file with a
// non-executable extension maps to ModeRegular.
func TestNormalizeModeForPath_RegularFile(t *testing.T) {
	got, err := NormalizeModeForPath(0o644, "note.txt")
	if err != nil {
		t.Fatalf("NormalizeModeForPath(0644, note.txt) error: %v", err)
	}
	if got != ModeRegular {
		t.Fatalf("NormalizeModeForPath(0644, note.txt) = %o, want %o", got, ModeRegular)
	}
}

// TestNormalizeModeForPath_ExecutableByExt verifies that on Windows, files
// with executable extensions are detected as executable even without the
// Unix executable bit.
func TestNormalizeModeForPath_ExecutableByExt(t *testing.T) {
	got, err := NormalizeModeForPath(0o644, "build.exe")
	if err != nil {
		t.Fatalf("NormalizeModeForPath(0644, build.exe) error: %v", err)
	}
	// On Windows, .exe should map to ModeExecutable; on Unix, ModeRegular.
	if isWindows {
		if got != ModeExecutable {
			t.Fatalf("NormalizeModeForPath(0644, build.exe) = %o, want %o (Windows)", got, ModeExecutable)
		}
	} else {
		if got != ModeRegular {
			t.Fatalf("NormalizeModeForPath(0644, build.exe) = %o, want %o (Unix)", got, ModeRegular)
		}
	}
}

// TestNormalizeModeForPath_SymlinkUnaffected verifies that symlinks are not
// affected by the path-based executable detection.
func TestNormalizeModeForPath_SymlinkUnaffected(t *testing.T) {
	got, err := NormalizeModeForPath(os.ModeSymlink|0o777, "link.exe")
	if err != nil {
		t.Fatalf("NormalizeModeForPath(symlink, link.exe) error: %v", err)
	}
	if got != ModeSymlink {
		t.Fatalf("NormalizeModeForPath(symlink, link.exe) = %o, want %o", got, ModeSymlink)
	}
}
