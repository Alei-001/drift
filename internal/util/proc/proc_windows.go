//go:build windows

package proc

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
}

const processQueryLimitedInformation = 0x1000

// processExistsWithStartTime reports whether the process with the given PID is
// currently alive. If startTime is non-zero (a Unix-nanosecond process
// creation timestamp), the function additionally verifies that the live
// process was started at approximately that time; a mismatch indicates the
// original process has exited and the PID has been reused by a new process.
// A startTime of zero skips the reuse check (used by callers that have no
// recorded start time, e.g. the watch daemon's PID file).
func ProcessExistsWithStartTime(pid int, startTime int64) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		// ERROR_ACCESS_DENIED means the process exists but we lack the
		// rights to query it (e.g. a higher-privilege system process).
		return err == syscall.ERROR_ACCESS_DENIED
	}
	defer syscall.CloseHandle(h)

	if startTime == 0 {
		return true
	}
	var creationTime, exitTime, kernelTime, userTime syscall.Filetime
	if err := syscall.GetProcessTimes(h, &creationTime, &exitTime, &kernelTime, &userTime); err != nil {
		return true // cannot verify; assume alive
	}
	actual := filetimeToUnixNano(creationTime)
	tolerance := int64(5 * time.Second)
	diff := actual - startTime
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

// filetimeToUnixNano converts a Windows FILETIME (100-nanosecond intervals
// since January 1, 1601) to a Unix-nanosecond timestamp (nanoseconds since
// January 1, 1970).
func filetimeToUnixNano(ft syscall.Filetime) int64 {
	// 116444736000000000 is the number of 100ns intervals between
	// 1601-01-01 and 1970-01-01.
	v := int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)
	v -= 116444736000000000
	v *= 100 // 100ns intervals -> nanoseconds
	return v
}

// currentProcessStartTime returns the current process's creation time as a
// Unix-nanosecond timestamp, used to detect PID reuse on Windows where PIDs
// are recycled aggressively.
func CurrentProcessStartTime() int64 {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	var creationTime, exitTime, kernelTime, userTime syscall.Filetime
	if err := syscall.GetProcessTimes(h, &creationTime, &exitTime, &kernelTime, &userTime); err != nil {
		return 0
	}
	return filetimeToUnixNano(creationTime)
}

func killProcess(pid int) error {
	return exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F").Run()
}
