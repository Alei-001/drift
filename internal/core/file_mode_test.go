package core

import "testing"

func TestFileMode_String(t *testing.T) {
	tests := []struct {
		name string
		mode FileMode
		want string
	}{
		{"regular", FileModeRegular, "regular"},
		{"regular with perm bits", FileModeRegular | 0o644, "regular"},
		{"dir", FileModeDir, "dir"},
		{"symlink", FileModeSymlink, "symlink"},
		{"unknown", FileMode(0), "unknown"},
		{"unknown high bits", FileMode(0o010000), "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mode.String(); got != tc.want {
				t.Errorf("FileMode(0%o).String() = %q, want %q", tc.mode, got, tc.want)
			}
		})
	}
}

func TestFileMode_IsRegular(t *testing.T) {
	if !FileModeRegular.IsRegular() {
		t.Error("FileModeRegular should be regular")
	}
	if !FileMode(FileModeRegular|0o755).IsRegular() {
		t.Error("regular with perm bits should be regular")
	}
	if FileModeDir.IsRegular() {
		t.Error("dir should not be regular")
	}
	if FileModeSymlink.IsRegular() {
		t.Error("symlink should not be regular")
	}
	if FileMode(0).IsRegular() {
		t.Error("unknown mode should not be regular")
	}
}

func TestFileMode_IsDir(t *testing.T) {
	if !FileModeDir.IsDir() {
		t.Error("FileModeDir should be dir")
	}
	if !FileMode(FileModeDir | 0o755).IsDir() {
		t.Error("dir with perm bits should be dir")
	}
	if FileModeRegular.IsDir() {
		t.Error("regular should not be dir")
	}
	if FileModeSymlink.IsDir() {
		t.Error("symlink should not be dir")
	}
}

func TestFileMode_IsSymlink(t *testing.T) {
	if !FileModeSymlink.IsSymlink() {
		t.Error("FileModeSymlink should be symlink")
	}
	if !FileMode(FileModeSymlink | 0o777).IsSymlink() {
		t.Error("symlink with perm bits should be symlink")
	}
	if FileModeRegular.IsSymlink() {
		t.Error("regular should not be symlink")
	}
	if FileModeDir.IsSymlink() {
		t.Error("dir should not be symlink")
	}
}

func TestFileMode_Constants(t *testing.T) {
	// FileModeMask must cover all type bits.
	if FileModeRegular&FileModeMask != FileModeRegular {
		t.Error("FileModeMask should preserve FileModeRegular type bits")
	}
	if FileModeDir&FileModeMask != FileModeDir {
		t.Error("FileModeMask should preserve FileModeDir type bits")
	}
	if FileModeSymlink&FileModeMask != FileModeSymlink {
		t.Error("FileModeMask should preserve FileModeSymlink type bits")
	}
	// Perm bits should be masked out by FileModeMask.
	perm := FileMode(0o644)
	if perm&FileModeMask != 0 {
		t.Errorf("perm bits 0%o should be masked out, got 0%o", perm, perm&FileModeMask)
	}
}
