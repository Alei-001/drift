package core

import "os"

const (
	ModeEmpty      uint32 = 0
	ModeDir        uint32 = 0040000
	ModeRegular    uint32 = 0100644
	ModeExecutable uint32 = 0100755
	ModeSymlink    uint32 = 0120000
)

func NormalizeMode(osMode os.FileMode) uint32 {
	if osMode&os.ModeSymlink != 0 {
		return ModeSymlink
	}
	if osMode.IsDir() {
		return ModeDir
	}
	if osMode.IsRegular() {
		if osMode&0111 != 0 {
			return ModeExecutable
		}
		return ModeRegular
	}
	return ModeRegular
}

func ToOSFileMode(mode uint32) os.FileMode {
	switch mode {
	case ModeDir:
		return os.ModeDir | 0755
	case ModeExecutable:
		return 0755
	case ModeSymlink:
		return os.ModeSymlink
	default:
		return 0644
	}
}
