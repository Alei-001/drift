package text

import "context"

// lcsCtxCheckInterval is how often lcsLastRow polls the context. The LCS
// inner loop is tight, so per-iteration checks would add overhead; polling
// every 1024 outer iterations keeps cancellation responsive.
const lcsCtxCheckInterval = 1024

// lcsLastRow computes the last row of the LCS DP table for a vs b.
// result[j] = LCS(a, b[0:j]). O(n) space. The context is polled every
// lcsCtxCheckInterval outer iterations via a counter to avoid per-iteration
// overhead on this tight inner loop.
func lcsLastRow(ctx context.Context, a, b []string) ([]int, error) {
	n := len(b)
	prev := make([]int, n+1)
	curr := make([]int, n+1)
	counter := 0
	for i := 0; i < len(a); i++ {
		if counter == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		counter++
		if counter >= lcsCtxCheckInterval {
			counter = 0
		}
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
	return prev, nil
}

// lcsLastRowReverse computes LCS(a, b[j:n]) for all j.
// result[j] = LCS(a, b[j:n]). O(n) space.
func lcsLastRowReverse(ctx context.Context, a, b []string) ([]int, error) {
	n := len(b)
	ra := make([]string, len(a))
	for i := range a {
		ra[i] = a[len(a)-1-i]
	}
	rb := make([]string, n)
	for i := range b {
		rb[i] = b[n-1-i]
	}
	row, err := lcsLastRow(ctx, ra, rb)
	if err != nil {
		return nil, err
	}
	// row[j] = LCS(ra, rb[0:j]) = LCS(a, b[n-j:n])
	// result[j] = LCS(a, b[j:n]) = row[n-j]
	result := make([]int, n+1)
	for j := 0; j <= n; j++ {
		result[j] = row[n-j]
	}
	return result, nil
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
