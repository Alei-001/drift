package cli

import (
	"os"

	"github.com/drift/drift/internal/core"
	"github.com/fatih/color"
)

// useColor reports whether colored output should be used.
// Color is disabled when:
//   - the --no-color flag is set
//   - the NO_COLOR environment variable is set (https://no-color.org/)
//   - stdout is not a terminal (e.g. piped to a file or another command)
func useColor() bool {
	if noColor {
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

// colorStatus returns the colored status code: A=green, M=yellow, D=red.
func colorStatus(s core.StatusCode) string {
	switch s {
	case core.Added:
		return colorGreen(string(s))
	case core.Modified:
		return colorYellow(string(s))
	case core.Deleted:
		return colorRed(string(s))
	default:
		return string(s)
	}
}

// shortHash returns the first 8 characters of a hash, or the full string
// if it is shorter than 8 characters. Prevents index-out-of-range panics.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// displayWidth returns the monospace terminal display width of s.
// CJK characters, Hangul, fullwidth forms, and emoji count as 2;
// everything else counts as 1.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if r < 0x1100 {
			w++
			continue
		}
		switch {
		case r <= 0x115f: // Hangul Jamo
			w += 2
		case r >= 0x2e80 && r <= 0xa4cf: // CJK radicals, unified ideographs, Yi
			w += 2
		case r >= 0xac00 && r <= 0xd7a3: // Hangul Syllables
			w += 2
		case r >= 0xf900 && r <= 0xfaff: // CJK Compatibility
			w += 2
		case r >= 0xfe10 && r <= 0xfe6f: // CJK Compat Forms, Small Variants
			w += 2
		case r >= 0xff01 && r <= 0xff60: // Fullwidth Forms
			w += 2
		case r >= 0xffe0 && r <= 0xffe6: // Fullwidth Signs
			w += 2
		case r >= 0x1f000: // Emoji, Symbols
			w += 2
		default:
			w++
		}
	}
	return w
}

// truncateByWidth truncates s so its display width does not exceed w.
// Appends "..." if truncation occurred.
func truncateByWidth(s string, w int) string {
	cur := 0
	for i, r := range s {
		dw := 1
		if r >= 0x1100 {
			switch {
			case r <= 0x115f,
				r >= 0x2e80 && r <= 0xa4cf,
				r >= 0xac00 && r <= 0xd7a3,
				r >= 0xf900 && r <= 0xfaff,
				r >= 0xfe10 && r <= 0xfe6f,
				r >= 0xff01 && r <= 0xff60,
				r >= 0xffe0 && r <= 0xffe6,
				r >= 0x1f000:
				dw = 2
			}
		}
		if cur+dw > w {
			if i >= 3 {
				return s[:i-3] + "..."
			}
			return s[:i] + "..."
		}
		cur += dw
	}
	return s
}

// colWidths holds computed column widths for formatted tables.
type colWidths struct {
	v []int
}

func newColWidths(n int) colWidths { return colWidths{v: make([]int, n)} }

func (c *colWidths) feed(i int, s string) {
	if dw := displayWidth(s); dw > c.v[i] {
		c.v[i] = dw
	}
}

func (c *colWidths) capAll(max int) {
	for i := range c.v {
		if c.v[i] > max {
			c.v[i] = max
		}
	}
}
