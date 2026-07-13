//go:build !windows

package porcelain

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// procClockTicksPerSec is the system clock tick rate used by /proc/<pid>/stat.
// On Linux this is virtually always 100 (USER_HZ); hard-coding avoids pulling
// in a unix-specific sysconf wrapper.
const procClockTicksPerSec = int64(100)

// processExistsWithStartTime reports whether the process with the given PID is
// currently alive. If startTime is non-zero (a Unix-nanosecond process
// creation timestamp), the function additionally verifies that the live
// process was started at approximately that time; a mismatch indicates the
// original process has exited and the PID has been reused by a new process.
// A startTime of zero skips the reuse check (used by callers that have no
// recorded start time, e.g. the watch daemon's PID file).
func processExistsWithStartTime(pid int, startTime int64) bool {
	// On Linux, /proc provides zombie state and starttime. macOS and other
	// systems lack /proc; fall back to kill -0.
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return processExistsViaSignal(pid)
	}
	s := string(statData)
	// /proc/<pid>/stat format: "pid (comm) state ppid ...". The comm field
	// may contain spaces and parens, so locate the LAST ')' to find state.
	idx := strings.LastIndexByte(s, ')')
	if idx < 0 || idx+2 >= len(s) {
		return processExistsViaSignal(pid)
	}
	state := s[idx+2]
	if state == 'Z' {
		// Zombie: the process has exited but not yet been reaped by its
		// parent. kill(pid, 0) succeeds for zombies, so Signal(0) alone is
		// insufficient to decide liveness.
		return false
	}
	if startTime > 0 {
		if actual, ok := parseProcStartTimeNano(s); ok {
			tolerance := int64(5 * time.Second)
			diff := actual - startTime
			if diff < 0 {
				diff = -diff
			}
			if diff > tolerance {
				return false // PID reused: different creation time
			}
		}
	}
	return true
}

// parseProcStartTimeNano parses /proc/<pid>/stat content and returns the
// process creation time as a Unix-nanosecond timestamp. The stat format is
// "pid (comm) state ppid pgrp ... starttime ..."; starttime (field 22
// counting pid and comm) is measured in clock ticks since boot. Conversion:
// boot_time + starttime_ticks / clk_tck.
func parseProcStartTimeNano(stat string) (int64, bool) {
	idx := strings.LastIndexByte(stat, ')')
	if idx < 0 {
		return 0, false
	}
	fields := strings.Fields(stat[idx+1:])
	// After ')', fields are: state ppid pgrp session tty_nr tpgid flags
	// minflt cminflt majflt cmajflt utime stime cutime cstime priority nice
	// num_threads itrealvalue starttime ... (starttime is the 20th field,
	// 0-indexed 19).
	if len(fields) < 20 {
		return 0, false
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return 0, false
	}
	uptime, err := readProcUptimeSeconds()
	if err != nil {
		return 0, false
	}
	bootNano := time.Now().UnixNano() - int64(uptime*1e9)
	return bootNano + ticks*1e9/procClockTicksPerSec, true
}

// readProcUptimeSeconds reads /proc/uptime and returns the system uptime in
// seconds (with sub-second precision).
func readProcUptimeSeconds() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("invalid /proc/uptime")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// processExistsViaSignal checks process liveness using kill(pid, 0). It is the
// fallback for platforms without /proc (e.g. macOS).
func processExistsViaSignal(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// currentProcessStartTime returns the current process's creation time as a
// Unix-nanosecond timestamp. On Linux this is read from /proc/self/stat; on
// macOS (no /proc) it returns 0, which disables the start-time reuse check.
func currentProcessStartTime() int64 {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}
	if start, ok := parseProcStartTimeNano(string(data)); ok {
		return start
	}
	return 0
}

func killProcess(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	return p.Signal(syscall.SIGTERM)
}
