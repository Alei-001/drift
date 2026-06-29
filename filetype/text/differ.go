package text

import (
	"strconv"
	"strings"
)

// Diff produces a unified diff between two text files using Myers diff.
func (e *TextEngine) Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error) {
	oldLines := splitLines(string(oldData))
	newLines := splitLines(string(newData))

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

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	// Normalize CRLF to LF
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Remove any remaining lone \r
	s = strings.ReplaceAll(s, "\r", "")
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n")
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

// myersDiff computes the edit script using the Myers diff algorithm.
// O(ND) time, O(D*(N+M)) space — a significant improvement over O(N*M) DP
// for inputs with small edit distance D.
func myersDiff(a, b []string) []editOp {
	m, n := len(a), len(b)
	if m == 0 && n == 0 {
		return nil
	}
	if m == 0 {
		script := make([]editOp, 0, n)
		for _, line := range b {
			script = append(script, editOp{editInsert, line})
		}
		return script
	}
	if n == 0 {
		script := make([]editOp, 0, m)
		for _, line := range a {
			script = append(script, editOp{editDelete, line})
		}
		return script
	}

	// V[k+offset] stores the furthest x reached on diagonal k = x - y.
	maxD := m + n
	offset := maxD
	v := make([]int, 2*maxD+1)

	// trace[d] is a snapshot of v before processing d, used for backtracking.
	trace := make([][]int, 0, maxD+1)

	d := 0
	for ; d <= maxD; d++ {
		vSnap := make([]int, len(v))
		copy(vSnap, v)
		trace = append(trace, vSnap)

		done := false
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1] // move down (insertion)
			} else {
				x = v[offset+k-1] + 1 // move right (deletion)
			}
			y := x - k
			for x < m && y < n && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= m && y >= n {
				done = true
				break
			}
		}
		if done {
			break
		}
	}

	// Backtrack through the trace to build the edit script in reverse.
	var script []editOp
	x, y := m, n
	for d = len(trace) - 1; d > 0; d-- {
		v = trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := v[offset+prevK]
		prevY := prevX - prevK

		// Emit diagonal matches (the "snake") in reverse order.
		for x > prevX && y > prevY {
			script = append(script, editOp{editMatch, a[x-1]})
			x--
			y--
		}

		// Emit the single edit (delete or insert) that links the snakes.
		if x == prevX {
			script = append(script, editOp{editInsert, b[y-1]})
		} else {
			script = append(script, editOp{editDelete, a[x-1]})
		}
		x = prevX
		y = prevY
	}
	// Emit any remaining leading matches.
	for x > 0 && y > 0 {
		script = append(script, editOp{editMatch, a[x-1]})
		x--
		y--
	}

	// Reverse to get forward order.
	for lo, hi := 0, len(script)-1; lo < hi; lo, hi = lo+1, hi-1 {
		script[lo], script[hi] = script[hi], script[lo]
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

		// Merge adjacent hunks whose context ranges overlap or touch.
		// When the gap between change regions is <= 2*contextSize, the
		// trailing context of one change overlaps the leading context of
		// the next; combine them into a single hunk instead.
		for {
			nextChange := hunkEnd
			for nextChange < len(script) && script[nextChange].action == editMatch {
				nextChange++
			}
			if nextChange >= len(script) {
				break
			}
			if nextChange-hunkEnd <= contextSize {
				changeEnd2 := nextChange
				for changeEnd2 < len(script) && script[changeEnd2].action != editMatch {
					changeEnd2++
				}
				hunkEnd = changeEnd2
				for i := 0; i < contextSize && hunkEnd < len(script); i++ {
					hunkEnd++
				}
			} else {
				break
			}
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
	sb.WriteString(strconv.Itoa(h.oldStart))
	sb.WriteByte(',')
	sb.WriteString(strconv.Itoa(h.oldCount))
	sb.WriteString(" +")
	sb.WriteString(strconv.Itoa(h.newStart))
	sb.WriteByte(',')
	sb.WriteString(strconv.Itoa(h.newCount))
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


