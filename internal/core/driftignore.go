package core

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type IgnoreMatcher struct {
	patterns []string
}

func LoadDriftIgnore(root string) *IgnoreMatcher {
	m := &IgnoreMatcher{}
	path := filepath.Join(root, ".driftignore")
	f, err := os.Open(path)
	if err != nil {
		return m
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		m.patterns = append(m.patterns, line)
	}
	return m
}

func (m *IgnoreMatcher) IsIgnored(relPath string) bool {
	name := filepath.Base(relPath)

	for _, pattern := range m.patterns {
		if matchPattern(pattern, relPath, name) {
			return true
		}
	}
	return false
}

func matchPattern(pattern, relPath, name string) bool {
	if pattern == "**" {
		return true
	}

	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, relPath)
	}

	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		if relPath == dirPattern || strings.HasPrefix(relPath, dirPattern+"/") {
			return true
		}
	}

	matched, _ := filepath.Match(pattern, name)
	if matched {
		return true
	}

	if relPath == pattern {
		return true
	}

	return false
}

func matchDoubleStar(pattern, relPath string) bool {
	// Issue 18: handle arbitrary number of "**" segments by splitting on "**"
	// and matching the literal parts in order against the path.
	parts := strings.Split(pattern, "**")
	if len(parts) < 2 {
		return false
	}

	// The first part must be a prefix of relPath (after trimming trailing /).
	first := strings.TrimSuffix(parts[0], "/")
	if first != "" {
		if relPath == first {
			// Path exactly equals the prefix; only matches if there are no
			// more parts requiring content after the last **.
			// Check that all remaining parts are empty.
			allEmpty := true
			for _, p := range parts[1:] {
				if strings.Trim(p, "/") != "" {
					allEmpty = false
					break
				}
			}
			return allEmpty
		}
		if !strings.HasPrefix(relPath, first+"/") {
			return false
		}
		relPath = relPath[len(first)+1:]
	}

	// For each middle part, it must appear as a path segment suffix somewhere
	// in the remaining path, and the next "**" can skip any number of dirs.
	for i := 1; i < len(parts)-1; i++ {
		middle := strings.Trim(parts[i], "/")
		if middle == "" {
			continue
		}
		// Find the middle literal in relPath as a path component.
		idx := findPathSegment(relPath, middle)
		if idx < 0 {
			return false
		}
		// Advance past the matched segment and the following "/".
		relPath = relPath[idx+len(middle):]
		relPath = strings.TrimPrefix(relPath, "/")
	}

	// The last part must be a suffix of the remaining path.
	last := strings.TrimPrefix(parts[len(parts)-1], "/")
	if last == "" {
		return true
	}
	if relPath == last || strings.HasSuffix(relPath, "/"+last) {
		return true
	}
	// Also allow glob matching on the basename.
	matched, _ := filepath.Match(last, filepath.Base(relPath))
	return matched
}

// findPathSegment returns the index of the first occurrence of segment as a
// complete path component in path (i.e., bounded by start/end or "/"), or -1.
func findPathSegment(path, segment string) int {
	if path == segment {
		return 0
	}
	prefix := segment + "/"
	if strings.HasPrefix(path, prefix) {
		return 0
	}
	mid := "/" + segment + "/"
	if idx := strings.Index(path, mid); idx >= 0 {
		return idx + 1
	}
	suffix := "/" + segment
	if strings.HasSuffix(path, suffix) {
		return len(path) - len(suffix)
	}
	return -1
}
