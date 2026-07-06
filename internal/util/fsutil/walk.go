package fsutil

import (
	"bufio"
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/your-org/drift/internal/util/glob"
)

// Walk walks the file tree rooted at root, invoking fn for every file not
// excluded by the ignore file. It is equivalent to WalkCtx with
// context.Background. ignoreFile defaults to ".driftignore" when empty.
// Symbolic links are not followed (filepath.WalkDir semantics). The .drift
// directory is always skipped.
func Walk(root, ignoreFile string, fn func(path string, info os.FileInfo) error) error {
	return WalkCtx(context.Background(), root, ignoreFile, fn)
}

// WalkCtx is the context-aware variant of Walk. It honors context
// cancellation: a cancelled context is surfaced before the walk starts and
// re-checked between entries, causing the walk to stop descending
// immediately. ignoreFile defaults to ".driftignore" when empty. Symbolic
// links are not followed, and the .drift directory is always skipped.
func WalkCtx(ctx context.Context, root, ignoreFile string, fn func(path string, info os.FileInfo) error) error {
	if ignoreFile == "" {
		ignoreFile = ".driftignore"
	}
	ignorePath := filepath.Join(root, ignoreFile)
	patterns, err := ReadIgnoreFile(ignorePath)
	if err != nil {
		return err
	}
	matchers := make([]*glob.Matcher, 0, len(patterns))
	for _, p := range patterns {
		m, err := glob.Compile(p)
		if err != nil {
			slog.Warn("invalid ignore pattern", "pattern", p, "error", err)
			continue
		}
		matchers = append(matchers, m)
	}
	// Surface an already-cancelled context before touching the filesystem so
	// callers do not pay for a WalkDir round-trip they never wanted.
	if err := ctx.Err(); err != nil {
		return err
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip files we can't access (permission denied, broken symlink, etc.)
			if os.IsPermission(err) || os.IsNotExist(err) {
				return nil
			}
			return err
		}

		// Honor context cancellation between entries. Returning the ctx error
		// here makes filepath.WalkDir stop descending immediately.
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if isDriftDir(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if isIgnored(rel, matchers) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(path, info)
	})
}

func isDriftDir(rel string) bool {
	return rel == ".drift" || strings.HasPrefix(rel, ".drift"+string(filepath.Separator))
}

// ReadIgnoreFile reads an ignore file and returns the active pattern lines.
// Comments (lines starting with '#') and blank lines are excluded.
// A UTF-8 BOM at the start of the file is stripped.
// Returns nil if the file does not exist.
func ReadIgnoreFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if firstLine {
			line = strings.TrimPrefix(line, "\xef\xbb\xbf")
			firstLine = false
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pattern := filepath.ToSlash(line)
		patterns = append(patterns, pattern)
	}
	return patterns, scanner.Err()
}

// isIgnored reports whether rel matches any of the precompiled ignore
// matchers. rel may use OS-native separators; it is normalized internally.
// Because the matchers are already compiled, this function cannot fail and
// is safe to call in a hot loop over many files.
func isIgnored(rel string, matchers []*glob.Matcher) bool {
	rel = filepath.ToSlash(rel)
	base := path.Base(rel)
	for _, m := range matchers {
		p := m.Pattern()
		// Anchored patterns (containing "/") must not match the bare
		// basename: "/secret.txt" must only match "secret.txt" at the
		// repository root, not "notes/secret.txt".
		if !strings.Contains(p, "/") {
			if m.Match(base) {
				return true
			}
		}
		if m.Match(rel) {
			return true
		}
	}
	return false
}
