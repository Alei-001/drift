package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// isInteractive returns true if stdin is a terminal (TTY).
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// confirmAction prompts the user for confirmation before a destructive action.
// If stdin is not a TTY (e.g. piped input or test), it auto-proceeds (returns true)
// to avoid blocking scripts. If force is true, it skips the prompt entirely.
//
// prompt is the question to ask (e.g. "Delete 3 files?").
// files is an optional list of affected files/objects to display before prompting.
func confirmAction(force bool, prompt string, files []string) bool {
	if force {
		return true
	}
	if !isInteractive() {
		// Non-interactive (pipe/script/test): auto-proceed.
		return true
	}

	// Display affected files if any.
	if len(files) > 0 {
		fmt.Println()
		limit := 20
		if len(files) < limit {
			limit = len(files)
		}
		for i := 0; i < limit; i++ {
			fmt.Printf("  %s\n", files[i])
		}
		if len(files) > limit {
			fmt.Printf("  ... and %d more\n", len(files)-limit)
		}
		fmt.Println()
	}

	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}
