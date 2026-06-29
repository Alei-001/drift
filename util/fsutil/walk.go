package fsutil

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func Walk(root string, fn func(path string, info os.FileInfo) error) error {
	patterns, err := readIgnorePatterns(root)
	if err != nil {
		return err
	}

	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files we can't access (permission denied, broken symlink, etc.)
			if os.IsPermission(err) || os.IsNotExist(err) {
				return nil
			}
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if isDriftDir(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if isIgnored(rel, info, patterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		return fn(path, info)
	})
}

func isDriftDir(rel string) bool {
	return rel == ".drift" || strings.HasPrefix(rel, ".drift"+string(filepath.Separator))
}

func readIgnorePatterns(root string) ([]string, error) {
	ignoreFile := filepath.Join(root, ".driftignore")
	f, err := os.Open(ignoreFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

func isIgnored(rel string, info os.FileInfo, patterns []string) bool {
	rel = filepath.ToSlash(rel)
	base := path.Base(rel)
	for _, p := range patterns {
		p = filepath.ToSlash(p)
		if match, _ := path.Match(p, base); match {
			return true
		}
		if match, _ := path.Match(p, rel); match {
			return true
		}
	}
	return false
}
