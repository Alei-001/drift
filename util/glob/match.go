package glob

import (
	"regexp"
	"strings"
)

// Match reports whether path matches the glob pattern.
// The pattern supports ** (recursive wildcard), * (single-level wildcard),
// ? (single character), and [...] (character classes).
// The path must use forward slashes.
func Match(pattern, path string) (bool, error) {
	regexStr := globToRegex(pattern)
	re, err := regexp.Compile("^" + regexStr + "$")
	if err != nil {
		return false, err
	}
	return re.MatchString(path), nil
}

func globToRegex(pattern string) string {
	pattern = strings.TrimPrefix(pattern, "/")
	var sb strings.Builder
	var lit strings.Builder
	flushLit := func() {
		if lit.Len() > 0 {
			sb.WriteString(regexp.QuoteMeta(lit.String()))
			lit.Reset()
		}
	}

	i := 0
	n := len(pattern)
	for i < n {
		c := pattern[i]
		switch c {
		case '*':
			flushLit()
			if i+1 < n && pattern[i+1] == '*' {
				if i+2 < n && pattern[i+2] == '/' {
					sb.WriteString("(.*/)?")
					i += 3
				} else {
					sb.WriteString(".*")
					i += 2
				}
			} else {
				sb.WriteString("[^/]*")
				i++
			}
		case '?':
			flushLit()
			sb.WriteString("[^/]")
			i++
		case '[':
			flushLit()
			j := i + 1
			negated := false
			if j < n && pattern[j] == '!' {
				negated = true
				j++
			}
			if j < n && pattern[j] == ']' {
				j++
			}
			for j < n && pattern[j] != ']' {
				j++
			}
			if j < n {
				// Translate the class to regex syntax. A leading '!' (glob
				// negation) becomes '^'; a leading ']' is a literal member.
				start := i + 1
				if negated {
					start++
				}
				sb.WriteByte('[')
				if negated {
					sb.WriteByte('^')
				}
				sb.WriteString(pattern[start:j])
				sb.WriteByte(']')
				i = j + 1
			} else {
				sb.WriteString("\\[")
				i++
			}
		default:
			lit.WriteByte(c)
			i++
		}
	}
	flushLit()
	return sb.String()
}
