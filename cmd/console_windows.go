//go:build windows

package cmd

import "syscall"

// cpUTF8 is the Windows code page identifier for UTF-8.
const cpUTF8 = 65001

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

// init sets the Windows console output code page to UTF-8 so that
// non-ASCII content (Chinese, Japanese, emoji, etc.) renders correctly.
// Without this, the default OEM code page mangles multi-byte output.
func init() {
	procSetConsoleOutputCP.Call(uintptr(cpUTF8))
}
