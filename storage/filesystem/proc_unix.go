//go:build !windows

package filesystem

import (
	"os"
	"syscall"
)

// processExists reports whether the process with the given pid is currently
// alive. It is the Unix variant of the implementation copied from the
// porcelain package to avoid a circular dependency.
func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
