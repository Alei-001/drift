package glob

import (
	"regexp"
	"strings"
)

// Matcher is a precompiled glob pattern that can be matched against many
// paths without recompiling the underlying regular expression.
// Create one with Compile and reuse it across many Match calls.
type Matcher struct {
	pattern string
	re      *regexp.Regexp
	dirOnly bool
}

// Compile compiles a glob pattern into a reusable Matcher.
// The pattern supports ** (recursive wildcard), * (single-level wildcard),
// ? (single character), and [...] (character classes).
// A trailing "/" marks the pattern as directory-only (gitignore semantics):
// the slash is stripped and IsDirOnly returns true so callers can skip
// non-directory entries.
// The path passed to Matcher.Match must use forward slashes.
func Compile(pattern string) (*Matcher, error) {
	dirOnly := false
	if strings.HasSuffix(pattern, "/") && len(pattern) > 1 {
		dirOnly = true
		pattern = pattern[:len(pattern)-1]
	}
	re, err := regexp.Compile("^" + globToRegex(pattern) + "$")
	if err != nil {
		return nil, err
	}
	return &Matcher{pattern: pattern, re: re, dirOnly: dirOnly}, nil
}

// Match reports whether path matches the precompiled pattern.
// The path must use forward slashes.
func (m *Matcher) Match(path string) bool {
	return m.re.MatchString(path)
}

// Pattern returns the original glob pattern the Matcher was compiled from.
// If the pattern had a trailing "/", it is stripped; use IsDirOnly to check.
func (m *Matcher) Pattern() string {
	return m.pattern
}

// IsDirOnly reports whether the pattern was suffixed with "/" (gitignore
// directory-only semantics). Callers should skip non-directory entries when
// this returns true.
func (m *Matcher) IsDirOnly() bool {
	return m.dirOnly
}

// Match reports whether path matches the glob pattern.
// The pattern supports ** (recursive wildcard), * (single-level wildcard),
// ? (single character), and [...] (character classes).
// The path must use forward slashes.
//
// For repeated matches against the same pattern, prefer Compile + Matcher.Match
// to avoid recompiling the regular expression on every call.
func Match(pattern, path string) (bool, error) {
	m, err := Compile(pattern)
	if err != nil {
		return false, err
	}
	return m.Match(path), nil
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
				// Backslashes inside the class are escaped to avoid
				// producing an invalid regex (e.g. [\] would close the
				// class prematurely).
				start := i + 1
				if negated {
					start++
				}
				sb.WriteByte('[')
				if negated {
					sb.WriteByte('^')
				}
				for k := start; k < j; k++ {
					c := pattern[k]
					if c == '\\' {
						sb.WriteString("\\\\")
					} else {
						sb.WriteByte(c)
					}
				}
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
