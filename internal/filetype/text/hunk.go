package text

import (
	"strconv"
	"strings"
)

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

		// A pure insertion hunk at the beginning of the file
		// (hunkStart==0, oldCount==0) uses oldStart=0 to match the
		// unified diff convention where a 0-count range starts at
		// line 0. Similarly for pure deletion (newCount==0).
		if hunkStart == 0 && oldCount == 0 {
			oldStart = 0
		}
		if hunkStart == 0 && newCount == 0 {
			newStart = 0
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
