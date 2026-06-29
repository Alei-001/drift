package text

import (
	"bufio"
	"bytes"
	"strings"
)

const defaultPreviewLines = 20

// Preview returns the first maxLines of text content.
func (e *TextEngine) Preview(data []byte, maxLines int) string {
	if maxLines <= 0 {
		maxLines = defaultPreviewLines
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	var lines []string
	for i := 0; i < maxLines && scanner.Scan(); i++ {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n")
}
