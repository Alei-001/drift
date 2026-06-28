package text

import (
	"strings"
)

const defaultPreviewLines = 20

// Preview returns the first maxLines of text content.
func (e *TextEngine) Preview(data []byte, maxLines int) string {
	if maxLines <= 0 {
		maxLines = defaultPreviewLines
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}
