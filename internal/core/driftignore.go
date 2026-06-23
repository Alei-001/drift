package core

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// IgnoreMatcher evaluates .driftignore patterns against repository-relative
// paths. The matching algorithm is ported from go-git's gitignore package,
// which itself mirrors canonical Git's wildmatch.c — so .driftignore supports
// the same pattern grammar as .gitignore: leading "!" for inclusion, trailing
// "/" for dir-only, "**" for arbitrary directory depth, bracket expressions
// with POSIX classes, and backslash escapes.
type IgnoreMatcher struct {
	patterns []Pattern
}

// Pattern is the interface implemented by each parsed .driftignore line.
// Mirrors go-git's gitignore.Pattern so the matcher can be reused verbatim.
type Pattern interface {
	// Match returns NoMatch, Exclude, or Include for the given slash-split
	// path. isDir reports whether the final path component is a directory.
	Match(p []string, isDir bool) MatchResult
}

// MatchResult is the outcome of a Pattern.Match call.
type MatchResult int

const (
	NoMatch MatchResult = iota
	Exclude
	Include
)

const (
	inclusionPrefix = "!"
	zeroToManyDirs  = "**"
	patternDirSep   = "/"
)

// driftignoreCache caches parsed IgnoreMatcher instances keyed by the
// canonical absolute path of the .driftignore file. The cached entry is
// invalidated when the file's modification time changes (B7).
// B10: bounded to maxDriftignoreEntries to avoid unbounded growth in
// long-running processes; eviction is simple clear-all on overflow since
// the typical working set is 1-2 entries per repository.
var driftignoreCache sync.Map // key: absPath → cachedIgnoreEntry
var driftignoreCacheLen int32 // atomic count of cached entries

const maxDriftignoreEntries = 64

type cachedIgnoreEntry struct {
	mtime time.Time
	m     *IgnoreMatcher
}

// LoadDriftIgnore reads .driftignore from root and returns a matcher.
// A missing file yields an empty matcher that ignores nothing.
// B7: caches the parsed result so repeated calls (e.g. from WalkWorkingDir
// in ComputeStatus and add) don't re-read and re-parse the file.
func LoadDriftIgnore(root string) *IgnoreMatcher {
	p := filepath.Join(root, ".driftignore")
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}

	// Check cache.
	fi, err := os.Stat(abs)
	if err != nil {
		// File missing — return empty matcher and clear cache.
		if _, ok := driftignoreCache.LoadAndDelete(abs); ok {
			atomic.AddInt32(&driftignoreCacheLen, -1)
		}
		return &IgnoreMatcher{}
	}
	mtime := fi.ModTime()
	if cached, ok := driftignoreCache.Load(abs); ok {
		entry := cached.(cachedIgnoreEntry)
		if entry.mtime.Equal(mtime) {
			return entry.m
		}
	}

	// Parse and cache.
	m := &IgnoreMatcher{}
	f, err := os.Open(abs)
	if err != nil {
		return m
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m.patterns = append(m.patterns, ParseDriftPattern(line))
	}

	// B10: bound the cache. If we'd exceed the limit, clear everything
	// (simple eviction — typical working set is tiny, so this is cheap
	// and avoids LRU bookkeeping).
	if _, exists := driftignoreCache.Load(abs); !exists {
		if atomic.LoadInt32(&driftignoreCacheLen) >= maxDriftignoreEntries {
			driftignoreCache.Range(func(k, _ any) bool {
				driftignoreCache.Delete(k)
				return true
			})
			atomic.StoreInt32(&driftignoreCacheLen, 0)
		}
		atomic.AddInt32(&driftignoreCacheLen, 1)
	}
	driftignoreCache.Store(abs, cachedIgnoreEntry{mtime: mtime, m: m})
	return m
}

// IsIgnored reports whether relPath is ignored by any pattern. relPath is
// a forward-slash path relative to the repository root. isDir defaults to
// false (file); callers that know the entry is a directory should use
// IsIgnoredDir.
func (m *IgnoreMatcher) IsIgnored(relPath string) bool {
	return m.match(relPath, false)
}

// IsIgnoredDir is IsIgnored for directory entries. Dir-only patterns
// (e.g. "build/") only match when isDir is true.
func (m *IgnoreMatcher) IsIgnoredDir(relPath string) bool {
	return m.match(relPath, true)
}

func (m *IgnoreMatcher) match(relPath string, isDir bool) bool {
	// Normalize to forward slashes and split into path components, the
	// same shape go-git's matcher expects.
	relPath = filepath.ToSlash(relPath)
	components := strings.Split(relPath, "/")

	ignored := false
	for _, p := range m.patterns {
		switch p.Match(components, isDir) {
		case Exclude:
			ignored = true
		case Include:
			// Later patterns win, but an explicit inclusion overrides any
			// preceding exclusion — mirror gitignore semantics.
			ignored = false
		}
	}
	return ignored
}

// ParseDriftPattern parses a single .driftignore line into a Pattern.
// Ported from go-git's gitignore.ParsePattern.
func ParseDriftPattern(p string) Pattern {
	res := &driftPattern{}

	if strings.HasPrefix(p, inclusionPrefix) {
		res.inclusion = true
		p = p[1:]
	}

	if !strings.HasSuffix(p, "\\ ") {
		p = strings.TrimRight(p, " ")
	}

	if strings.HasSuffix(p, patternDirSep) {
		res.dirOnly = true
		p = p[:len(p)-1]
	}

	if strings.Contains(p, patternDirSep) {
		res.isGlob = true
	}

	res.pattern = strings.Split(p, patternDirSep)
	return res
}

type driftPattern struct {
	pattern   []string
	inclusion bool
	dirOnly   bool
	isGlob    bool
}

func (p *driftPattern) Match(path []string, isDir bool) MatchResult {
	if len(path) == 0 {
		return NoMatch
	}

	if p.isGlob {
		if !p.globMatch(path, isDir) {
			return NoMatch
		}
	} else if !p.simpleNameMatch(path, isDir) {
		return NoMatch
	}

	if p.inclusion {
		return Include
	}
	return Exclude
}

func (p *driftPattern) simpleNameMatch(path []string, isDir bool) bool {
	for i, name := range path {
		if !wildmatch(p.pattern[0], name) {
			continue
		}
		if p.dirOnly && !isDir && i == len(path)-1 {
			return false
		}
		return true
	}
	return false
}

func (p *driftPattern) globMatch(path []string, isDir bool) bool {
	matched := false
	canTraverse := false
	trailingStar := false
	for i, pattern := range p.pattern {
		if pattern == "" {
			canTraverse = false
			continue
		}
		if pattern == zeroToManyDirs {
			if i == len(p.pattern)-1 {
				// Trailing ** matches everything remaining.
				if len(path) > 0 || isDir {
					matched = true
					trailingStar = true
				}
				break
			}
			canTraverse = true
			continue
		}
		if len(path) == 0 {
			return false
		}
		if canTraverse {
			canTraverse = false
			for len(path) > 0 {
				e := path[0]
				path = path[1:]
				if wildmatch(pattern, e) {
					matched = true
					break
				} else if len(path) == 0 {
					matched = false
				}
			}
		} else {
			if !wildmatch(pattern, path[0]) {
				return false
			}
			matched = true
			path = path[1:]
			if len(path) == 0 && i < len(p.pattern)-1 {
				matched = false
			}
		}
	}
	if matched && p.dirOnly && !isDir && (len(path) == 0 || trailingStar) {
		matched = false
	}
	return matched
}

// Keep path imported for future use; mirrors go-git's package layout where
// path joining helpers live alongside the matcher.
var _ = path.Join

// The wildmatch implementation below ports the matcher from canonical Git's
// wildmatch.c, via go-git's gitignore package. The algorithm is preserved
// exactly; only the package boundary changed.

const (
	wmMatch           = 0
	wmNoMatch         = 1
	wmAbortAll        = -1
	wmAbortToStarStar = -2
)

const (
	wmCasefold = 1
	wmPathname = 2
)

// wildmatch reports whether text matches the wildcard pattern. The gitignore
// matcher splits paths on '/' before dispatching, so this always operates on
// a single pattern/text segment with flags=0.
func wildmatch(pattern, text string) bool {
	return dowild(pattern, text, 0) == wmMatch
}

// dowild walks pattern and text in lock-step, recursing at each '*' to try
// every text suffix and propagating wmMatch / wmNoMatch / wmAbortAll /
// wmAbortToStarStar back up so callers can prune work the same way upstream
// wildmatch.c does.
func dowild(p, text string, flags int) int {
	pi, ti := 0, 0
	for pi < len(p) {
		pCh := p[pi]
		var tCh byte
		atEndOfText := ti >= len(text)
		if !atEndOfText {
			tCh = text[ti]
		}
		if atEndOfText && pCh != '*' {
			return wmAbortAll
		}
		if flags&wmCasefold != 0 && isASCIIUpper(tCh) {
			tCh += 'a' - 'A'
		}
		if flags&wmCasefold != 0 && isASCIIUpper(pCh) {
			pCh += 'a' - 'A'
		}

		switch pCh {
		case '\\':
			if pi+1 >= len(p) {
				return wmNoMatch
			}
			pi++
			pCh = p[pi]
			if tCh != pCh {
				return wmNoMatch
			}
			pi++
			ti++
		case '?':
			if flags&wmPathname != 0 && tCh == '/' {
				return wmNoMatch
			}
			pi++
			ti++
		case '*':
			pi++
			var matchSlash bool
			if pi < len(p) && p[pi] == '*' {
				prevPi := pi
				for pi < len(p) && p[pi] == '*' {
					pi++
				}
				switch {
				case flags&wmPathname == 0:
					matchSlash = true
				case (prevPi < 2 || p[prevPi-2] == '/') &&
					(pi >= len(p) || p[pi] == '/' ||
						(pi+1 < len(p) && p[pi] == '\\' && p[pi+1] == '/')):
					if pi < len(p) && p[pi] == '/' &&
						dowild(p[pi+1:], text[ti:], flags) == wmMatch {
						return wmMatch
					}
					matchSlash = true
				}
			} else {
				matchSlash = flags&wmPathname == 0
			}

			if pi >= len(p) {
				if !matchSlash && strings.IndexByte(text[ti:], '/') >= 0 {
					return wmAbortToStarStar
				}
				return wmMatch
			} else if !matchSlash && p[pi] == '/' {
				slash := strings.IndexByte(text[ti:], '/')
				if slash < 0 {
					return wmAbortAll
				}
				ti += slash
				pi++
				ti++
				continue
			}

			for {
				if ti >= len(text) {
					return wmAbortAll
				}
				tCh = text[ti]
				if !isGlobSpecial(p[pi]) {
					pCh = p[pi]
					if flags&wmCasefold != 0 && isASCIIUpper(pCh) {
						pCh += 'a' - 'A'
					}
					for ti < len(text) {
						tCh = text[ti]
						if !matchSlash && tCh == '/' {
							break
						}
						if flags&wmCasefold != 0 && isASCIIUpper(tCh) {
							tCh += 'a' - 'A'
						}
						if tCh == pCh {
							break
						}
						ti++
					}
					if ti >= len(text) || tCh != pCh {
						if matchSlash {
							return wmAbortAll
						}
						return wmAbortToStarStar
					}
				}
				matched := dowild(p[pi:], text[ti:], flags)
				if matched != wmNoMatch {
					if !matchSlash || matched != wmAbortToStarStar {
						return matched
					}
				} else if !matchSlash && tCh == '/' {
					return wmAbortToStarStar
				}
				ti++
			}
		case '[':
			pi++
			if pi >= len(p) {
				return wmAbortAll
			}
			pCh = p[pi]
			if pCh == '^' {
				pCh = '!'
			}
			negated := pCh == '!'
			if negated {
				pi++
				if pi >= len(p) {
					return wmAbortAll
				}
				pCh = p[pi]
			}
			var prevCh byte
			matched := false
			for {
				switch {
				case pCh == '\\':
					pi++
					if pi >= len(p) {
						return wmAbortAll
					}
					pCh = p[pi]
					if tCh == pCh {
						matched = true
					}
				case pCh == '-' && prevCh != 0 &&
					pi+1 < len(p) && p[pi+1] != ']':
					pi++
					pCh = p[pi]
					if pCh == '\\' {
						pi++
						if pi >= len(p) {
							return wmAbortAll
						}
						pCh = p[pi]
					}
					if tCh <= pCh && tCh >= prevCh {
						matched = true
					} else if flags&wmCasefold != 0 && isASCIILower(tCh) {
						tUpper := tCh - ('a' - 'A')
						if tUpper <= pCh && tUpper >= prevCh {
							matched = true
						}
					}
					pCh = 0
				case pCh == '[' && pi+1 < len(p) && p[pi+1] == ':':
					s := pi + 2
					pi = s
					for pi < len(p) && p[pi] != ']' {
						pi++
					}
					if pi >= len(p) {
						return wmAbortAll
					}
					nameLen := pi - s - 1
					if nameLen < 0 || p[pi-1] != ':' {
						pi = s - 2
						pCh = '['
						if tCh == pCh {
							matched = true
						}
						break
					}
					classMatched, valid := matchPOSIXClass(p[s:pi-1], tCh, flags)
					if !valid {
						return wmAbortAll
					}
					if classMatched {
						matched = true
					}
					pCh = 0
				default:
					if tCh == pCh {
						matched = true
					}
				}
				prevCh = pCh
				pi++
				if pi >= len(p) {
					return wmAbortAll
				}
				if p[pi] == ']' {
					break
				}
				pCh = p[pi]
			}
			if matched == negated ||
				(flags&wmPathname != 0 && tCh == '/') {
				return wmNoMatch
			}
			pi++
			ti++
		default:
			if tCh != pCh {
				return wmNoMatch
			}
			pi++
			ti++
		}
	}

	if ti < len(text) {
		return wmNoMatch
	}
	return wmMatch
}

// isGlobSpecial mirrors is_glob_special() from upstream ctype.c.
func isGlobSpecial(c byte) bool {
	switch c {
	case '*', '?', '[', '\\':
		return true
	}
	return false
}

// matchPOSIXClass evaluates a [:name:] character-class entry within a bracket
// expression. Classification is ASCII-only to mirror sane-ctype.h.
func matchPOSIXClass(name string, ch byte, flags int) (matched, valid bool) {
	switch name {
	case "alnum":
		return isASCIIAlpha(ch) || isASCIIDigit(ch), true
	case "alpha":
		return isASCIIAlpha(ch), true
	case "blank":
		return ch == ' ' || ch == '\t', true
	case "cntrl":
		return ch < 0x20 || ch == 0x7f, true
	case "digit":
		return isASCIIDigit(ch), true
	case "graph":
		return ch > ' ' && ch < 0x7f, true
	case "lower":
		return ch >= 'a' && ch <= 'z', true
	case "print":
		return ch >= ' ' && ch < 0x7f, true
	case "punct":
		return isASCIIPunct(ch), true
	case "space":
		return ch == ' ' || ch == '\t' || ch == '\n' ||
			ch == '\v' || ch == '\f' || ch == '\r', true
	case "upper":
		if ch >= 'A' && ch <= 'Z' {
			return true, true
		}
		if flags&wmCasefold != 0 && isASCIILower(ch) {
			return true, true
		}
		return false, true
	case "xdigit":
		return isASCIIDigit(ch) || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F'), true
	default:
		return false, false
	}
}

func isASCIIAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isASCIIDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isASCIIUpper(ch byte) bool {
	return ch >= 'A' && ch <= 'Z'
}

func isASCIILower(ch byte) bool {
	return ch >= 'a' && ch <= 'z'
}

func isASCIIPunct(ch byte) bool {
	return (ch >= '!' && ch <= '/') ||
		(ch >= ':' && ch <= '@') ||
		(ch >= '[' && ch <= '`') ||
		(ch >= '{' && ch <= '~')
}
