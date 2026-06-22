package core

import (
	"fmt"
	"os"
)

const (
	ModeEmpty      uint32 = 0
	ModeDir        uint32 = 0o040000
	ModeRegular    uint32 = 0o100644
	ModeExecutable uint32 = 0o100755
	ModeSymlink    uint32 = 0o120000
)

// ErrUnsupportedFileType is returned when a file type cannot be represented
// in the Drift object model (e.g., sockets, pipes, device files).
var ErrUnsupportedFileType = fmt.Errorf("unsupported file type (only regular files, directories, and symlinks are supported)")

// NormalizeMode converts an os.FileMode to a Drift FileMode. Returns
// ErrUnsupportedFileType for file types that Drift cannot represent
// (sockets, pipes, devices), instead of silently treating them as regular.
func NormalizeMode(osMode os.FileMode) (uint32, error) {
	if osMode&os.ModeSymlink != 0 {
		return ModeSymlink, nil
	}
	if osMode.IsDir() {
		return ModeDir, nil
	}
	if osMode.IsRegular() {
		if osMode&0o111 != 0 {
			return ModeExecutable, nil
		}
		return ModeRegular, nil
	}
	return 0, ErrUnsupportedFileType
}

// ToOSFileMode converts a Drift FileMode to an os.FileMode suitable for
// os.WriteFile/os.Chmod. Note: for symlinks, callers must use os.Symlink
// rather than os.WriteFile — see writeBlobToWorktree.
func ToOSFileMode(mode uint32) os.FileMode {
	switch mode {
	case ModeDir:
		return os.ModeDir | 0o755
	case ModeExecutable:
		return 0o755
	case ModeSymlink:
		return os.ModeSymlink | 0o777
	default:
		return 0o644
	}
}

// IsMalformed reports whether a mode is not a recognized Drift FileMode.
func IsMalformed(mode uint32) bool {
	switch mode {
	case ModeEmpty, ModeDir, ModeRegular, ModeExecutable, ModeSymlink:
		return false
	}
	return true
}
