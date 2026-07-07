package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
)

// formatBranchColumn formats the branch-tip list for the log table column.
// Multiple tips are joined with ","; overflows are truncated as
// "name1,name2,+N" so the user knows how many were hidden. Returns "" when the
// slice is empty (inherited commits show no branch).
func formatBranchColumn(names []string) string {
	if len(names) == 0 {
		return ""
	}
	// Try to fit all names.
	joined := strings.Join(names, ",")
	if len([]rune(joined)) <= branchColWidth {
		return joined
	}
	// Truncate progressively: keep as many leading names as fit, then append
	// ",+N" to indicate how many were dropped.
	runes := []rune(joined)
	if len(runes) <= branchColWidth {
		return string(runes)
	}
	// Reserve room for the "+N" suffix.
	for keep := len(names) - 1; keep >= 1; keep-- {
		prefix := strings.Join(names[:keep], ",")
		suffix := fmt.Sprintf(",+%d", len(names)-keep)
		if len([]rune(prefix))+len([]rune(suffix)) <= branchColWidth {
			return prefix + suffix
		}
	}
	// Even one name + "+N" doesn't fit: hard-truncate the first name.
	if len(runes) > branchColWidth {
		return string(runes[:branchColWidth-1]) + "..."
	}
	return joined
}

// includesAutoSaves reports whether any entry in the list has the auto-save
// message prefix. Used to decide whether the label should mention auto-saves.
func includesAutoSaves(snaps []*core.SnapshotSummary) bool {
	for _, s := range snaps {
		if strings.HasPrefix(s.Message, porcelain.AutoSavePrefix) {
			return true
		}
	}
	return false
}

// mergeTags merges embedded snapshot tags with tag refs (added later via
// 'drift tag add'), dedups case-sensitively, drops empty entries, and sorts
// alphabetically for stable display. Returns a new slice; nil inputs yield an
// empty (non-nil) slice.
func mergeTags(embedded, refs []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(embedded)+len(refs))
	for _, t := range embedded {
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, t := range refs {
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// formatTagColumn renders the tag list for the log table column. Tags are
// wrapped in brackets, e.g. "[v1.0]" or "[v1.0,v2.0]". When the joined list
// exceeds tagMaxLen runes, it is truncated to "[<first>,+N]" so the user knows
// additional tags exist. Returns "" when the merged list is empty.
func formatTagColumn(embedded, refs []string) string {
	tags := mergeTags(embedded, refs)
	if len(tags) == 0 {
		return ""
	}
	inner := strings.Join(tags, ",")
	if len([]rune(inner)) <= tagMaxLen {
		return "[" + inner + "]"
	}
	// Try to fit as many leading tags as possible with a ",+N" suffix.
	runes := []rune(inner)
	for keep := len(tags) - 1; keep >= 1; keep-- {
		prefix := strings.Join(tags[:keep], ",")
		suffix := fmt.Sprintf(",+%d", len(tags)-keep)
		if len([]rune(prefix))+len([]rune(suffix)) <= tagMaxLen {
			return "[" + prefix + suffix + "]"
		}
	}
	// Even one name + ",+N" doesn't fit: hard-truncate the first tag.
	if len(runes) > tagMaxLen {
		return "[" + string(runes[:tagMaxLen-1]) + "...]"
	}
	return "[" + inner + "]"
}
