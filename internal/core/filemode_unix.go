//go:build !windows

package core

// isWindows is a compile-time constant indicating whether the build target
// is Windows. Used by tests to verify platform-specific behavior.
const isWindows = false

// isExecutableByPath always returns false on Unix, where the executable
// bit is determined by filesystem permissions (checked in NormalizeMode).
func isExecutableByPath(name string) bool {
	return false
}
