package cli

import (
	"os"

	"github.com/fatih/color"
)

// useColor reports whether colored output should be used.
// Color is disabled when:
//   - the --no-color flag is set
//   - the NO_COLOR environment variable is set (https://no-color.org/)
//   - stdout is not a terminal (e.g. piped to a file or another command)
func useColor() bool {
	if globalNoColor {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return false
	}
	return true
}

// colorGreen wraps s with green ANSI codes when color is enabled.
func colorGreen(s string) string {
	if !useColor() {
		return s
	}
	return color.New(color.FgGreen).Sprint(s)
}

// colorYellow wraps s with yellow ANSI codes when color is enabled.
func colorYellow(s string) string {
	if !useColor() {
		return s
	}
	return color.New(color.FgYellow).Sprint(s)
}

// colorRed wraps s with red ANSI codes when color is enabled.
func colorRed(s string) string {
	if !useColor() {
		return s
	}
	return color.New(color.FgRed).Sprint(s)
}

// colorCyan wraps s with cyan ANSI codes when color is enabled.
func colorCyan(s string) string {
	if !useColor() {
		return s
	}
	return color.New(color.FgCyan).Sprint(s)
}

// colorGray wraps s with gray (bright black) ANSI codes when color is enabled.
func colorGray(s string) string {
	if !useColor() {
		return s
	}
	return color.New(color.FgHiBlack).Sprint(s)
}
