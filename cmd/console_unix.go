//go:build !windows

package cmd

// On non-Windows platforms UTF-8 is the default console encoding, so no
// setup is needed. This file exists solely to balance console_windows.go
// via build constraints.
