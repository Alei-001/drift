package text

import "context"

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
// diff to avoid excessive runtime. The context is checked before dispatching
// so a cancelled caller aborts before the DP work begins.
func myersDiff(ctx context.Context, a, b []string) ([]editOp, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(a) > maxDiffLines || len(b) > maxDiffLines {
		return coarseDiff(a, b), nil
	}
	if !hasCommonLines(a, b) {
		return coarseDiff(a, b), nil
	}
	return hirschbergDiff(ctx, a, b)
}

// hirschbergDiff computes the edit script using Hirschberg's
// divide-and-conquer algorithm. Splits a at its midpoint, finds the
// optimal split in b via LCS row computation, then recurses on both
// halves. O(N) space per level. The context is checked before each
// recursive call so a cancelled caller aborts the recursion promptly.
func hirschbergDiff(ctx context.Context, a, b []string) ([]editOp, error) {
	m, n := len(a), len(b)
	if m == 0 && n == 0 {
		return nil, nil
	}
	if m == 0 {
		script := make([]editOp, 0, n)
		for _, line := range b {
			script = append(script, editOp{editInsert, line})
		}
		return script, nil
	}
	if n == 0 {
		script := make([]editOp, 0, m)
		for _, line := range a {
			script = append(script, editOp{editDelete, line})
		}
		return script, nil
	}
	// Base case: small enough for classic Myers
	if m <= 16 || n <= 16 {
		return myersDiffBasic(ctx, a, b)
	}

	mid := m / 2
	lf, err := lcsLastRow(ctx, a[:mid], b)
	if err != nil {
		return nil, err
	}
	lb, err := lcsLastRowReverse(ctx, a[mid:], b)
	if err != nil {
		return nil, err
	}

	bestJ := 0
	bestSum := -1
	for j := 0; j <= n; j++ {
		s := lf[j] + lb[j]
		if s > bestSum {
			bestSum = s
			bestJ = j
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	left, err := hirschbergDiff(ctx, a[:mid], b[:bestJ])
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	right, err := hirschbergDiff(ctx, a[mid:], b[bestJ:])
	if err != nil {
		return nil, err
	}
	return append(left, right...), nil
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
// is negligible. The context is checked at the top of the d-loop.
func myersDiffBasic(ctx context.Context, a, b []string) ([]editOp, error) {
	m, n := len(a), len(b)
	if m == 0 && n == 0 {
		return nil, nil
	}
	if m == 0 {
		script := make([]editOp, 0, n)
		for _, line := range b {
			script = append(script, editOp{editInsert, line})
		}
		return script, nil
	}
	if n == 0 {
		script := make([]editOp, 0, m)
		for _, line := range a {
			script = append(script, editOp{editDelete, line})
		}
		return script, nil
	}

	maxD := m + n
	offset := maxD
	v := make([]int, 2*maxD+1)
	trace := make([][]int, 0, maxD+1)

	d := 0
	for ; d <= maxD; d++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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

	return script, nil
}
