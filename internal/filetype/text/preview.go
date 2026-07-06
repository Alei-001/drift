package text

import (
	"bufio"
	"io"
	"strings"
)

const defaultPreviewLines = 20

// Preview returns the first maxLines of text content, read streaming from
// reader. Only the requested number of lines are consumed, so previewing a
// huge text file does not load it whole.
func (e *TextEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
	_ = size
	if maxLines <= 0 {
		maxLines = defaultPreviewLines
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for i := 0; i < maxLines && scanner.Scan(); i++ {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}
