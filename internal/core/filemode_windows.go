//go:build windows

package core

import "strings"

// isWindows is a compile-time constant indicating whether the build target
// is Windows. Used by tests to verify platform-specific behavior.
const isWindows = true

// isExecutableByPath returns true for files that should be treated as
// executable on Windows, where the filesystem does not store the Unix
// executable permission bit. Files with known executable extensions are
// detected so the executable mode is preserved cross-platform.
func isExecutableByPath(name string) bool {
	ext := strings.ToLower(name)
	for _, e := range windowsExecutableExts {
		if strings.HasSuffix(ext, e) {
			return true
		}
	}
	return false
}

// windowsExecutableExts lists file extensions that are conventionally
// executable on Windows.
var windowsExecutableExts = []string{
	".exe", ".bat", ".cmd", ".com",
	".ps1", ".vbs", ".vba", ".wsf",
	".sh", ".py", ".rb", ".pl",
	".msi",
}
