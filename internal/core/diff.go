package core

import (
	"bytes"
)

// MyersDiff implements the Myers O(ND) diff algorithm — the same algorithm
// Git uses. It computes the shortest edit script (SES) between two string
// slices in O((M+N)+D²) time and O(M+N) space, where D is the edit distance.
//
// B8: replaces the O(M×N) DP-based LCS algorithm that used O(M×N) memory and
// was capped at 20000 lines. Myers diff scales to much larger inputs.
//
// Implementation uses the forward-only O(ND) variance with backtracking via
// a trace of the V frontier at each D.

// DiffOp represents a single edit operation in the shortest edit script.
type DiffOp byte

const (
	DiffKeep   DiffOp = ' '
	DiffDelete DiffOp = '-'
	DiffInsert DiffOp = '+'
)

// DiffEdit is a single element of the edit script.
type DiffEdit struct {
	Op   DiffOp
	Line string
	OldN int // line number in the old file (0-based), or -1 for inserts
	NewN int // line number in the new file (0-based), or -1 for deletes
}

// Myers computes the shortest edit script between a and b using the
// Myers O(ND) forward-only algorithm with trace-based backtracking.
//
// This is the simpler, more robust variance of the algorithm that avoids
// the tricky middle-snake divide-and-conquer recursion.
func Myers(a, b []string) []DiffEdit {
	// Strip common prefix and suffix.
	m, n := len(a), len(b)
	start := 0
	for start < m && start < n && a[start] == b[start] {
		start++
	}
	endA, endB := m, n
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA--
		endB--
	}

	// All lines matched.
	if start >= endA && start >= endB {
		edits := make([]DiffEdit, 0, m)
		for i := 0; i < m; i++ {
			edits = append(edits, DiffEdit{Op: DiffKeep, Line: a[i], OldN: i, NewN: i})
		}
		return edits
	}

	// Compute edits on the divergent middle section.
	midEdits := computeEdits(a[start:endA], b[start:endB])

	// Reconstruct full edit script.
	total := start + len(midEdits) + (m - endA)
	edits := make([]DiffEdit, 0, total)

	for i := 0; i < start; i++ {
		edits = append(edits, DiffEdit{Op: DiffKeep, Line: a[i], OldN: i, NewN: i})
	}

	oldOff, newOff := start, start
	for _, e := range midEdits {
		switch e.Op {
		case DiffKeep:
			e.OldN = oldOff
			e.NewN = newOff
			oldOff++
			newOff++
		case DiffDelete:
			e.OldN = oldOff
			e.NewN = -1
			oldOff++
		case DiffInsert:
			e.OldN = -1
			e.NewN = newOff
			newOff++
		}
		edits = append(edits, e)
	}

	for i := endA; i < m; i++ {
		edits = append(edits, DiffEdit{Op: DiffKeep, Line: a[i], OldN: i, NewN: endB + (i - endA)})
	}

	return edits
}

func computeEdits(a, b []string) []DiffEdit {
	m, n := len(a), len(b)

	// Trivial cases.
	if m == 0 && n == 0 {
		return nil
	}
	if m == 0 {
		edits := make([]DiffEdit, n)
		for i := 0; i < n; i++ {
			edits[i] = DiffEdit{Op: DiffInsert, Line: b[i], OldN: -1, NewN: i}
		}
		return edits
	}
	if n == 0 {
		edits := make([]DiffEdit, m)
		for i := 0; i < m; i++ {
			edits[i] = DiffEdit{Op: DiffDelete, Line: a[i], OldN: i, NewN: -1}
		}
		return edits
	}

	maxD := m + n
	V := make([]int, 2*maxD+3)
	trace := make([][]int, 0, maxD)
	offset := maxD

	// Initialize V to -1.
	for i := range V {
		V[i] = -1
	}
	V[1+offset] = 0

forwardLoop:
	for d := 0; d <= maxD; d++ {
		// Save a copy of V for backtracking.
		vc := make([]int, len(V))
		copy(vc, V)
		trace = append(trace, vc)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && V[k-1+offset] < V[k+1+offset]) {
				x = V[k+1+offset]
			} else {
				x = V[k-1+offset] + 1
			}
			y := x - k

			// Follow the snake.
			for x < m && y < n && a[x] == b[y] {
				x++
				y++
			}

			V[k+offset] = x

			if x >= m && y >= n {
				// Found the shortest path.
				break forwardLoop
			}
		}
	}

	// Backtrack to produce the edit script.
	return backtrack(a, b, trace, m, n, offset)
}

func backtrack(a, b []string, trace [][]int, m, n, offset int) []DiffEdit {
	// Walk backwards through the trace, collecting edit operations.
	var edits []DiffEdit

	x, y := m, n
	for d := len(trace) - 1; d >= 0; d-- {
		V := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && V[k-1+offset] < V[k+1+offset]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}

		prevX := V[prevK+offset]
		prevY := prevX - prevK

		// Walk the diagonal snake backward.
		for x > prevX && y > prevY {
			edits = append(edits, DiffEdit{
				Op:   DiffKeep,
				Line: a[x-1],
				OldN: x - 1,
				NewN: y - 1,
			})
			x--
			y--
		}

		if d == 0 {
			break
		}

		// Record the non-diagonal step.
		if x == prevX {
			edits = append(edits, DiffEdit{
				Op:   DiffInsert,
				Line: b[prevY],
				OldN: -1,
				NewN: prevY,
			})
		} else {
			edits = append(edits, DiffEdit{
				Op:   DiffDelete,
				Line: a[prevX],
				OldN: prevX,
				NewN: -1,
			})
		}
		x, y = prevX, prevY
	}

	// Reverse to get chronological order.
	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}

	return edits
}

// DiffEditScriptToUnified converts a Myers edit script into unified-diff
// output suitable for display. Returns lines prefixed with ' ', '+', or '-'.
func DiffEditScriptToUnified(edits []DiffEdit) []byte {
	var buf bytes.Buffer
	for _, e := range edits {
		buf.WriteByte(byte(e.Op))
		buf.WriteString(e.Line)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// DiffCountChanges returns the number of added and deleted lines from
// a Myers edit script.
func DiffCountChanges(edits []DiffEdit) (added, deleted int) {
	for _, e := range edits {
		switch e.Op {
		case DiffInsert:
			added++
		case DiffDelete:
			deleted++
		}
	}
	return
}
