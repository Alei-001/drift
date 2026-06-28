package text

import (
	"strings"
)

// Diff produces a unified diff between two text files using LCS.
func (e *TextEngine) Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error) {
	oldLines := splitLines(string(oldData))
	newLines := splitLines(string(newData))

	if len(oldLines) == 0 && len(newLines) == 0 {
		return "", nil
	}

	lcs := computeLCS(oldLines, newLines)
	script := buildEditScript(oldLines, newLines, lcs)

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

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n")
}

type lcsEntry struct{ oldIdx, newIdx int }

func computeLCS(a, b []string) []lcsEntry {
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return nil
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else if dp[i-1][j] >= dp[i][j-1] {
				dp[i][j] = dp[i-1][j]
			} else {
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	var result []lcsEntry
	i, j := m, n
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			result = append(result, lcsEntry{i - 1, j - 1})
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	for lo, hi := 0, len(result)-1; lo < hi; lo, hi = lo+1, hi-1 {
		result[lo], result[hi] = result[hi], result[lo]
	}

	return result
}

type editAction int

const (
	editMatch  editAction = 0
	editDelete editAction = 1
	editInsert editAction = 2
)

type editOp struct {
	action editAction
	line   string
}

func buildEditScript(oldLines, newLines []string, lcs []lcsEntry) []editOp {
	var script []editOp
	oldIdx, newIdx := 0, 0
	for _, e := range lcs {
		for oldIdx < e.oldIdx {
			script = append(script, editOp{editDelete, oldLines[oldIdx]})
			oldIdx++
		}
		for newIdx < e.newIdx {
			script = append(script, editOp{editInsert, newLines[newIdx]})
			newIdx++
		}
		script = append(script, editOp{editMatch, oldLines[oldIdx]})
		oldIdx++
		newIdx++
	}
	for oldIdx < len(oldLines) {
		script = append(script, editOp{editDelete, oldLines[oldIdx]})
		oldIdx++
	}
	for newIdx < len(newLines) {
		script = append(script, editOp{editInsert, newLines[newIdx]})
		newIdx++
	}
	return script
}

func isAllMatch(script []editOp) bool {
	for _, op := range script {
		if op.action != editMatch {
			return false
		}
	}
	return true
}

type hunk struct {
	oldStart, oldCount int
	newStart, newCount int
	lines              []string
}

func groupIntoHunks(script []editOp) []hunk {
	const contextSize = 3
	var hunks []hunk
	idx := 0

	for idx < len(script) {
		// Find the next changed line
		changeStart := idx
		for changeStart < len(script) && script[changeStart].action == editMatch {
			changeStart++
		}
		if changeStart >= len(script) {
			break
		}

		// Hunk includes context lines before the first change
		hunkStart := changeStart
		for i := 0; i < contextSize && hunkStart > 0; i++ {
			hunkStart--
		}

		// Find the end of the changes (the next match after all changes)
		changeEnd := changeStart
		for changeEnd < len(script) && script[changeEnd].action != editMatch {
			changeEnd++
		}

		// Include context lines after
		hunkEnd := changeEnd
		for i := 0; i < contextSize && hunkEnd < len(script); i++ {
			hunkEnd++
		}

		// Calculate line numbers
		oldStart := 1 // 1-based
		newStart := 1
		for i := 0; i < hunkStart; i++ {
			switch script[i].action {
			case editDelete, editMatch:
				oldStart++
			}
		}
		for i := 0; i < hunkStart; i++ {
			switch script[i].action {
			case editInsert, editMatch:
				newStart++
			}
		}

		// Build lines and counts
		var lines []string
		oldCount := 0
		newCount := 0
		for i := hunkStart; i < hunkEnd && i < len(script); i++ {
			op := script[i]
			switch op.action {
			case editDelete:
				lines = append(lines, "-"+op.line)
				oldCount++
			case editInsert:
				lines = append(lines, "+"+op.line)
				newCount++
			case editMatch:
				lines = append(lines, " "+op.line)
				oldCount++
				newCount++
			}
		}

		hunks = append(hunks, hunk{
			oldStart: oldStart,
			oldCount: oldCount,
			newStart: newStart,
			newCount: newCount,
			lines:    lines,
		})

		idx = hunkEnd
	}

	return hunks
}

func (h hunk) String() string {
	var sb strings.Builder
	sb.WriteString("@@ -")
	sb.WriteString(itoa(h.oldStart))
	sb.WriteByte(',')
	sb.WriteString(itoa(h.oldCount))
	sb.WriteString(" +")
	sb.WriteString(itoa(h.newStart))
	sb.WriteByte(',')
	sb.WriteString(itoa(h.newCount))
	sb.WriteString(" @@")
	if len(h.lines) > 0 {
		sb.WriteByte('\n')
	}
	for _, line := range h.lines {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
