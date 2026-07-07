package fsutil

import (
	"fmt"
	"os"
	"strings"
)

// ListIgnoreRules reads the ignore file at path and returns all rules.
// Returns nil if the file does not exist.
func ListIgnoreRules(ignoreFilePath string) ([]string, error) {
	return ReadIgnoreFile(ignoreFilePath)
}

// AddIgnoreRules appends the given rules to the ignore file, skipping
// duplicates and preserving existing order. Returns the rules actually
// added.
func AddIgnoreRules(ignoreFilePath string, rules []string) ([]string, error) {
	data, err := os.ReadFile(ignoreFilePath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	existing, _ := ReadIgnoreFile(ignoreFilePath)
	set := make(map[string]bool)
	for _, r := range existing {
		set[r] = true
	}
	var added []string
	seen := make(map[string]bool)
	for _, p := range rules {
		p = strings.TrimSpace(p)
		if p == "" || set[p] || seen[p] {
			continue
		}
		seen[p] = true
		set[p] = true
		added = append(added, p)
	}
	var buf strings.Builder
	buf.Write(data)
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		buf.WriteByte('\n')
	}
	for _, p := range added {
		buf.WriteString(p)
		buf.WriteByte('\n')
	}
	if err := WriteFileAtomic(ignoreFilePath, []byte(buf.String()), DefaultFilePerm); err != nil {
		return nil, err
	}
	return added, nil
}

// RemoveIgnoreRule removes the first rule matching the given pattern from
// the ignore file. Returns an error if the rule is not found.
func RemoveIgnoreRule(ignoreFilePath string, pattern string) error {
	data, err := os.ReadFile(ignoreFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pattern '%s' not found", pattern)
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) == pattern {
			found = true
			continue
		}
		out = append(out, line)
	}
	if !found {
		return fmt.Errorf("pattern '%s' not found", pattern)
	}
	return WriteFileAtomic(ignoreFilePath, []byte(strings.Join(out, "\n")), DefaultFilePerm)
}
