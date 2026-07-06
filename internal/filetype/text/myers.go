package text

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

const maxDiffLines = 500000

// myersDiff computes the edit script using the Myers diff algorithm with
// Hirschberg's divide-and-conquer strategy for linear space. O(N*M) time,
// O(N) space. Inputs exceeding maxDiffLines fall back to coarse prefix/suffix
// diff to avoid excessive runtime.
func myersDiff(a, b []string) []editOp {
	if len(a) > maxDiffLines || len(b) > maxDiffLines {
		return coarseDiff(a, b)
	}
	if !hasCommonLines(a, b) {
		return coarseDiff(a, b)
	}
	return hirschbergDiff(a, b)
}

// hirschbergDiff computes the edit script using Hirschberg's
// divide-and-conquer algorithm. Splits a at its midpoint, finds the
// optimal split in b via LCS row computation, then recurses on both
// halves. O(N) space per level.
func hirschbergDiff(a, b []string) []editOp {
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
	// Base case: small enough for classic Myers
	if m <= 16 || n <= 16 {
		return myersDiffBasic(a, b)
	}

	mid := m / 2
	lf := lcsLastRow(a[:mid], b)
	lb := lcsLastRowReverse(a[mid:], b)

	bestJ := 0
	bestSum := -1
	for j := 0; j <= n; j++ {
		s := lf[j] + lb[j]
		if s > bestSum {
			bestSum = s
			bestJ = j
		}
	}

	left := hirschbergDiff(a[:mid], b[:bestJ])
	right := hirschbergDiff(a[mid:], b[bestJ:])
	return append(left, right...)
}

// lcsLastRow computes the last row of the LCS DP table for a vs b.
// result[j] = LCS(a, b[0:j]). O(n) space.
func lcsLastRow(a, b []string) []int {
	n := len(b)
	prev := make([]int, n+1)
	curr := make([]int, n+1)
	for i := 0; i < len(a); i++ {
		curr[0] = 0
		for j := 0; j < n; j++ {
			if a[i] == b[j] {
				curr[j+1] = prev[j] + 1
			} else if prev[j+1] >= curr[j] {
				curr[j+1] = prev[j+1]
			} else {
				curr[j+1] = curr[j]
			}
		}
		prev, curr = curr, prev
	}
	return prev
}

// lcsLastRowReverse computes LCS(a, b[j:n]) for all j.
// result[j] = LCS(a, b[j:n]). O(n) space.
func lcsLastRowReverse(a, b []string) []int {
	n := len(b)
	ra := make([]string, len(a))
	for i := range a {
		ra[i] = a[len(a)-1-i]
	}
	rb := make([]string, n)
	for i := range b {
		rb[i] = b[n-1-i]
	}
	row := lcsLastRow(ra, rb)
	// row[j] = LCS(ra, rb[0:j]) = LCS(a, b[n-j:n])
	// result[j] = LCS(a, b[j:n]) = row[n-j]
	result := make([]int, n+1)
	for j := 0; j <= n; j++ {
		result[j] = row[n-j]
	}
	return result
}

// hasCommonLines checks if any line in a appears in b.
// O(m+n) time, O(n) space.
func hasCommonLines(a, b []string) bool {
	set := make(map[string]struct{}, len(b))
	for _, line := range b {
		set[line] = struct{}{}
	}
	for _, line := range a {
		if _, ok := set[line]; ok {
			return true
		}
	}
	return false
}

// coarseDiff produces a simple prefix/suffix diff for oversized inputs
// or inputs with no common lines.
func coarseDiff(a, b []string) []editOp {
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix &&
		a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	var script []editOp
	for i := 0; i < prefix; i++ {
		script = append(script, editOp{editMatch, a[i]})
	}
	for i := prefix; i < len(a)-suffix; i++ {
		script = append(script, editOp{editDelete, a[i]})
	}
	for i := prefix; i < len(b)-suffix; i++ {
		script = append(script, editOp{editInsert, b[i]})
	}
	for i := len(a) - suffix; i < len(a); i++ {
		script = append(script, editOp{editMatch, a[i]})
	}
	return script
}

// myersDiffBasic computes the edit script using the classic Myers diff
// algorithm. O(ND) time, O(D*(N+M)) space. Used as the base case for
// Hirschberg recursion where inputs are small enough that trace storage
// is negligible.
func myersDiffBasic(a, b []string) []editOp {
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

	maxD := m + n
	offset := maxD
	v := make([]int, 2*maxD+1)
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
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
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

		for x > prevX && y > prevY {
			script = append(script, editOp{editMatch, a[x-1]})
			x--
			y--
		}

		if x == prevX {
			script = append(script, editOp{editInsert, b[y-1]})
		} else {
			script = append(script, editOp{editDelete, a[x-1]})
		}
		x = prevX
		y = prevY
	}
	for x > 0 && y > 0 {
		script = append(script, editOp{editMatch, a[x-1]})
		x--
		y--
	}

	for lo, hi := 0, len(script)-1; lo < hi; lo, hi = lo+1, hi-1 {
		script[lo], script[hi] = script[hi], script[lo]
	}

	return script
}
