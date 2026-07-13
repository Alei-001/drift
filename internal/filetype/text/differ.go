package text

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
)

// ErrLineTooLong is returned by readLines when a single line exceeds the
// scanner's maximum buffer size (scannerMaxBufSize). Callers should treat
// this as "files differ" rather than a hard error, since the file is likely
// binary or minified and cannot be meaningfully line-diffed.
var ErrLineTooLong = errors.New("line too long for text diff")

// scannerInitialBufSize and scannerMaxBufSize size the bufio.Scanner buffer
// for reading potentially long lines in text files.
const (
	scannerInitialBufSize = 64 * 1024
	scannerMaxBufSize     = 1024 * 1024
)

// Diff produces a unified diff between two text files using Myers diff.
// Content is read streaming from oldReader/newReader via a line scanner;
// the Myers algorithm still requires both line arrays in memory, but the
// file bytes are never buffered whole. The context is threaded into the
// Myers/Hirschberg loops so a cancelled context aborts the diff promptly.
//
// Inputs exceeding maxDiffLines fall back to a simple "files differ"
// message to avoid the O(N*M) Myers DP, which would be prohibitively slow
// for very large files. Identical content is short-circuited before the
// threshold check so that large but unchanged files produce no diff.
func (e *TextEngine) Diff(ctx context.Context, oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldLines, err := readLines(oldReader)
	if err != nil {
		if errors.Is(err, ErrLineTooLong) {
			return buildFilesDifferDiff(oldPath, newPath), nil
		}
		return "", err
	}
	newLines, err := readLines(newReader)
	if err != nil {
		if errors.Is(err, ErrLineTooLong) {
			return buildFilesDifferDiff(oldPath, newPath), nil
		}
		return "", err
	}

	if len(oldLines) == 0 && len(newLines) == 0 {
		return "", nil
	}

	// Fast path: identical content produces no diff, even for large inputs.
	if linesEqual(oldLines, newLines) {
		return "", nil
	}

	// Fallback for oversized inputs: a simple "files differ" message avoids
	// the O(N*M) Myers DP, which would be prohibitively slow for very large
	// files.
	if len(oldLines) > maxDiffLines || len(newLines) > maxDiffLines {
		return buildFilesDifferDiff(oldPath, newPath), nil
	}

	script, err := myersDiff(ctx, oldLines, newLines)
	if err != nil {
		return "", err
	}

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

// buildFilesDifferDiff returns a minimal unified-diff stub indicating that
// the files differ without computing a detailed line-level diff. Used when
// the input exceeds maxDiffLines.
func buildFilesDifferDiff(oldPath, newPath string) string {
	var sb strings.Builder
	sb.WriteString("--- ")
	sb.WriteString(oldPath)
	sb.WriteByte('\n')
	sb.WriteString("+++ ")
	sb.WriteString(newPath)
	sb.WriteByte('\n')
	sb.WriteString("files differ (too large for detailed diff)\n")
	return sb.String()
}

// linesEqual reports whether two line slices are identical. This fast O(N)
// check short-circuits identical large files before the maxDiffLines
// threshold fallback.
func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// readLines reads all lines from r using a bufio.Scanner with a custom
// split function that handles \r\n, \n, and \r line endings. A generous
// 1 MiB buffer is used so long lines do not trip the scanner's default
// 64 KiB limit. If a single line still exceeds the buffer, ErrLineTooLong
// is returned so the caller can fall back to a "files differ" result.
func readLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, scannerInitialBufSize), scannerMaxBufSize)
	scanner.Split(scanLines)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, ErrLineTooLong
		}
		return nil, err
	}
	return lines, nil
}

// scanLines is a bufio.SplitFunc that recognizes three line terminators:
// \r\n (CRLF), \n (LF), and bare \r (classic Mac CR). The terminator is
// consumed but not included in the returned token, mirroring
// bufio.ScanLines behavior while adding bare \r support.
func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	nlIdx := bytes.IndexByte(data, '\n')
	crIdx := bytes.IndexByte(data, '\r')
	// If \n appears at or before \r (or there is no \r), split on \n
	// and strip a preceding \r if present (CRLF case).
	if nlIdx >= 0 && (crIdx < 0 || nlIdx <= crIdx) {
		end := nlIdx
		if end > 0 && data[end-1] == '\r' {
			end--
		}
		return nlIdx + 1, data[0:end], nil
	}
	// \r appears before \n (or there is no \n). If \r is followed by \n,
	// consume both (CRLF); otherwise treat bare \r as a line terminator.
	if crIdx >= 0 {
		if crIdx+1 == len(data) {
			// \r is the last byte. Request more data unless at EOF.
			if !atEOF {
				return 0, nil, nil
			}
			return crIdx + 1, data[0:crIdx], nil
		}
		if data[crIdx+1] == '\n' {
			return crIdx + 2, data[0:crIdx], nil
		}
		return crIdx + 1, data[0:crIdx], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func isAllMatch(script []editOp) bool {
	for _, op := range script {
		if op.action != editMatch {
			return false
		}
	}
	return true
}
