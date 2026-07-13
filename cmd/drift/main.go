package main

import (
	"fmt"
	"os"

	"github.com/Alei-001/drift/cmd"
)

func main() {
	// Recover from a failed binary upgrade before doing anything else:
	// if a previous `drift upgrade` crashed between renaming the running
	// binary to <exe>.old and moving the new binary into place, the
	// executable may be missing. Restore it from the .old backup so the
	// command can run normally. This is best-effort and silent on
	// success; failures are reported to stderr but never fatal.
	recoverFromFailedUpgrade()
	os.Exit(cmd.Execute())
}

// recoverFromFailedUpgrade detects the half-replaced state left by a
// crashed upgrade and repairs it. The two-step rename in
// version.replaceExecutable first moves the running binary to
// "<exe>.old", then moves the new binary to <exe>. If the process dies
// between those two renames, <exe> is missing and <exe>.old is the
// previous (still-runnable) binary. On the next launch we rename it
// back. If both files exist (the upgrade completed but the .old cleanup
// failed, common on Windows), the leftover .old is removed.
func recoverFromFailedUpgrade() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	oldPath := exePath + ".old"
	if _, err := os.Stat(oldPath); err != nil {
		// No .old file: nothing to recover.
		return
	}
	if _, err := os.Stat(exePath); err != nil {
		// exe missing, .old present: restore from backup.
		if err := os.Rename(oldPath, exePath); err == nil {
			fmt.Fprintln(os.Stderr, "drift: recovered from failed upgrade")
		}
		return
	}
	// Both present: .old is a leftover from a completed upgrade.
	// Best-effort remove; ignore errors (Windows may still hold it).
	_ = os.Remove(oldPath)
}
