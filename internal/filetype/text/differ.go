package text

import (
	"bufio"
	"io"
	"strings"
)

// Diff produces a unified diff between two text files using Myers diff.
// Content is read streaming from oldReader/newReader via a line scanner;
// the Myers algorithm still requires both line arrays in memory, but the
// file bytes are never buffered whole.
func (e *TextEngine) Diff(oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldLines, err := readLines(oldReader)
	if err != nil {
		return "", err
	}
	newLines, err := readLines(newReader)
	if err != nil {
		return "", err
	}

	if len(oldLines) == 0 && len(newLines) == 0 {
		return "", nil
	}

	script := myersDiff(oldLines, newLines)

	if isAllMatch(script) {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("--- ")
	sb.WriteString(oldPath)
	sb.WriteByte('\n')
	sb.WriteString("+++ ")
	sb.WriteString(newPath)
	sb.WriteByte('\n')

	hunks := groupIntoHunks(script)
	for _, h := range hunks {
		sb.WriteString(h.String())
	}

	return sb.String(), nil
}

// readLines reads all lines from r using a bufio.Scanner. ScanLines strips
// the trailing "\n" or "\r\n" terminator, which normalizes CRLF line
// endings for free. A generous 1 MiB buffer is used so long lines do not
// trip the scanner's default 64 KiB limit.
func readLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func isAllMatch(script []editOp) bool {
	for _, op := range script {
		if op.action != editMatch {
			return false
		}
	}
	return true
}
