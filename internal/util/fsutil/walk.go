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

	"github.com/Alei-001/drift/internal/util/glob"
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
//
// Nested ignore files in subdirectories are honored: a .driftignore in a
// subdirectory adds rules scoped to that subtree, following gitignore
// semantics. Patterns starting with "!" negate a previous match, and a
// trailing "/" restricts a pattern to directories only.
func WalkCtx(ctx context.Context, root, ignoreFile string, fn func(path string, info os.FileInfo) error) error {
	if ignoreFile == "" {
		ignoreFile = ".driftignore"
	}
	ignorePath := filepath.Join(root, ignoreFile)
	patterns, err := ReadIgnoreFile(ignorePath)
	if err != nil {
		return err
	}
	stack := []ignoreLayer{{relDir: "", rules: compileIgnoreRules(patterns)}}
	// Surface an already-cancelled context before touching the filesystem so
	// callers do not pay for a WalkDir round-trip they never wanted.
	if err := ctx.Err(); err != nil {
		return err
	}

	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				// Log permission errors so the user knows files were
				// skipped — a silent skip could lead to incomplete
				// snapshots without any indication.
				slog.Warn("skip unreadable path during walk", "path", p, "error", err)
				return nil
			}
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		// Pop ignore layers for directories we have left.
		stack = popStaleLayers(stack, rel)

		if isDriftDir(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// For directories (other than root), check for a nested ignore file
		// and push its rules onto the stack so they apply to the subtree.
		if d.IsDir() && rel != "." {
			nestedPath := filepath.Join(p, ignoreFile)
			if _, statErr := os.Stat(nestedPath); statErr == nil {
				np, readErr := ReadIgnoreFile(nestedPath)
				if readErr != nil {
					slog.Warn("read nested ignore file", "path", nestedPath, "error", readErr)
				} else {
					stack = append(stack, ignoreLayer{relDir: rel, rules: compileIgnoreRules(np)})
				}
			}
		}

		if isIgnored(rel, d.IsDir(), stack) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(p, info)
	})
}

func isDriftDir(rel string) bool {
	return rel == ".drift" || strings.HasPrefix(rel, ".drift/")
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

// ignoreRule pairs a compiled glob matcher with a negation flag. A negated
// rule (pattern prefixed with "!") un-ignores a previously matched path,
// following gitignore semantics.
type ignoreRule struct {
	matcher *glob.Matcher
	negated bool
}

// compileIgnoreRule compiles a single pattern into an ignoreRule. A leading
// "!" marks the rule as a negation (the "!" is stripped before compilation).
func compileIgnoreRule(pattern string) (ignoreRule, error) {
	negated := false
	if strings.HasPrefix(pattern, "!") {
		negated = true
		pattern = pattern[1:]
	}
	m, err := glob.Compile(pattern)
	if err != nil {
		return ignoreRule{}, err
	}
	return ignoreRule{matcher: m, negated: negated}, nil
}

// compileIgnoreRules compiles a list of raw patterns into ignoreRules,
// skipping invalid patterns with a warning.
func compileIgnoreRules(patterns []string) []ignoreRule {
	rules := make([]ignoreRule, 0, len(patterns))
	for _, p := range patterns {
		rule, err := compileIgnoreRule(p)
		if err != nil {
			slog.Warn("invalid ignore pattern", "pattern", p, "error", err)
			continue
		}
		rules = append(rules, rule)
	}
	return rules
}

// ignoreLayer holds the ignore rules from a single .driftignore file, scoped
// to the directory relDir (relative to the walk root, forward slashes).
// The root layer has relDir "".
type ignoreLayer struct {
	relDir string
	rules  []ignoreRule
}

// popStaleLayers removes layers from the top of the stack whose relDir is
// not an ancestor of rel. The root layer (relDir "") is never popped.
func popStaleLayers(stack []ignoreLayer, rel string) []ignoreLayer {
	for len(stack) > 1 {
		top := stack[len(stack)-1]
		if rel == top.relDir || strings.HasPrefix(rel, top.relDir+"/") {
			break
		}
		stack = stack[:len(stack)-1]
	}
	return stack
}

// isIgnored reports whether rel (forward slashes, relative to walk root)
// is ignored by the stack of ignore layers. Rules are evaluated in order
// across all layers; the last matching rule wins. Negated rules ("!" prefix)
// un-ignore; directory-only rules (trailing "/") are skipped for files.
func isIgnored(rel string, isDir bool, layers []ignoreLayer) bool {
	ignored := false
	for _, layer := range layers {
		layerRel := rel
		if layer.relDir != "" {
			prefix := layer.relDir + "/"
			if !strings.HasPrefix(rel, prefix) {
				continue
			}
			layerRel = rel[len(prefix):]
		}
		base := path.Base(layerRel)
		for _, rule := range layer.rules {
			if rule.matcher.IsDirOnly() && !isDir {
				continue
			}
			p := rule.matcher.Pattern()
			matched := false
			if !strings.Contains(p, "/") {
				matched = rule.matcher.Match(base)
			}
			if !matched {
				matched = rule.matcher.Match(layerRel)
			}
			if matched {
				ignored = !rule.negated
			}
		}
	}
	return ignored
}
