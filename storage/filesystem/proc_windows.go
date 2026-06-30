//go:build windows

package filesystem

import (
	"fmt"
	"os/exec"
	"strings"
)

// processExists reports whether the process with the given pid is currently
// alive. It is the Windows variant of the implementation copied from the
// porcelain package to avoid a circular dependency.
func processExists(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH", "/FO", "CSV")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return false
	}
	return strings.Contains(output, ",")
}
