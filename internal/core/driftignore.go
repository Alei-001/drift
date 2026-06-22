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
	parts := strings.Split(pattern, "**")
	if len(parts) == 2 {
		prefix := parts[0]
		suffix := parts[1]

		if prefix == "" {
			suffix = strings.TrimPrefix(suffix, "/")
			if suffix == "" {
				return true
			}
			if relPath == suffix || strings.HasSuffix(relPath, "/"+suffix) {
				return true
			}
			matched, _ := filepath.Match(suffix, filepath.Base(relPath))
			if matched {
				return true
			}
			return false
		}

		if suffix == "" {
			prefix = strings.TrimSuffix(prefix, "/")
			if strings.HasPrefix(relPath, prefix) {
				return true
			}
			return false
		}

		prefix = strings.TrimSuffix(prefix, "/")
		suffix = strings.TrimPrefix(suffix, "/")

		if !strings.HasPrefix(relPath, prefix) {
			return false
		}
		remaining := strings.TrimPrefix(relPath, prefix)
		remaining = strings.TrimPrefix(remaining, "/")

		if remaining == suffix || strings.HasSuffix(remaining, "/"+suffix) {
			return true
		}
		matched, _ := filepath.Match(suffix, filepath.Base(remaining))
		if matched {
			return true
		}
		return false
	}

	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		if relPath == suffix || strings.HasSuffix(relPath, "/"+suffix) {
			return true
		}
		matched, _ := filepath.Match(suffix, filepath.Base(relPath))
		if matched {
			return true
		}
	}

	return false
}
