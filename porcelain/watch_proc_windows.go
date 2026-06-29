//go:build windows

package porcelain

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}

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

func killProcess(pid int) error {
	return exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F").Run()
}
