package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/storage"
)

// logJSONEntry is one row of the 'drift log --json' output. The schema
// matches docs/cli-design.md: tags is an array, branch is a single string.
type logJSONEntry struct {
	ID      string   `json:"id"`
	Time    string   `json:"time"`
	Message string   `json:"message"`
	Tags    []string `json:"tags"`
	Branch  string   `json:"branch"`
	Changes string   `json:"changes"`
}

// logJSONData wraps the snapshot list in the envelope's data field.
type logJSONData struct {
	Snapshots []logJSONEntry `json:"snapshots"`
}

// logJSONMode emits the snapshot history as a JSON envelope. The schema
// matches docs/cli-design.md: each entry has id, time, message, tags (array),
// branch (string), and changes formatted as "+A ~M -D". No status line is
// printed to stdout — only the JSON envelope is emitted.
func logJSONMode(ctx context.Context, store storage.Storer, snapshots []*core.SnapshotSummary, branchMap map[string][]string) error {
	entries := make([]logJSONEntry, 0, len(snapshots))
	for _, s := range snapshots {
		add, mod, del := countSnapshotChanges(ctx, store, s)
		tags := make([]string, 0, len(s.Tags))
		for _, t := range s.Tags {
			if t != "" {
				tags = append(tags, t)
			}
		}
		entry := logJSONEntry{
			ID:      s.ShortID(),
			Time:    time.Unix(s.Timestamp, 0).Format("2006-01-02T15:04:05"),
			Message: s.Message,
			Tags:    tags,
			Changes: fmt.Sprintf("+%d ~%d -%d", add, mod, del),
		}
		if branches, ok := branchMap[s.ID.Hash.String()]; ok && len(branches) > 0 {
			entry.Branch = branches[0]
		}
		entries = append(entries, entry)
	}
	return outputJSON(JSONEnvelope{
		Command: "log",
		Status:  "ok",
		Data:    logJSONData{Snapshots: entries},
	})
}

// logDetailJSONFile describes a single added or modified file in --detail
// --json output. Lines is only set for modified text files.
type logDetailJSONFile struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	Lines int    `json:"lines,omitempty"`
}

// logDetailJSONChanges groups the added/modified/deleted file sets.
type logDetailJSONChanges struct {
	Added    []logDetailJSONFile `json:"added"`
	Modified []logDetailJSONFile `json:"modified"`
	Deleted  []string            `json:"deleted"`
}

// logDetailJSONSummary is the per-snapshot change tally.
type logDetailJSONSummary struct {
	Total    int `json:"total"`
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

// logDetailJSONData is the data payload of the --detail --json envelope.
type logDetailJSONData struct {
	ID      string               `json:"id"`
	Time    string               `json:"time"`
	Message string               `json:"message"`
	Changes logDetailJSONChanges `json:"changes"`
	Summary logDetailJSONSummary `json:"summary"`
}

// logDetailJSONMode emits a single snapshot's file-change detail as a JSON
// envelope, mirroring the human --detail view: added/modified file entries
// with sizes and line counts (for modified text files), deleted paths, and a
// summary tally.
func logDetailJSONMode(ctx context.Context, store storage.Storer, snapshot *core.Snapshot, added, modified []core.FileEntry, deleted []string) error {
	addEntries := make([]logDetailJSONFile, 0, len(added))
	for _, f := range added {
		addEntries = append(addEntries, logDetailJSONFile{Path: f.Path, Size: f.Size})
	}
	modEntries := make([]logDetailJSONFile, 0, len(modified))
	for _, f := range modified {
		entry := logDetailJSONFile{Path: f.Path, Size: f.Size}
		if lines := countLinesFromChunks(ctx, store, f); lines > 0 {
			entry.Lines = lines
		}
		modEntries = append(modEntries, entry)
	}
	delEntries := deleted
	if delEntries == nil {
		delEntries = []string{}
	}
	data := logDetailJSONData{
		ID:      snapshot.ShortID(),
		Time:    time.Unix(snapshot.Timestamp, 0).Format("2006-01-02T15:04:05"),
		Message: snapshot.Message,
		Changes: logDetailJSONChanges{
			Added:    addEntries,
			Modified: modEntries,
			Deleted:  delEntries,
		},
		Summary: logDetailJSONSummary{
			Total:    len(added) + len(modified) + len(deleted),
			Added:    len(added),
			Modified: len(modified),
			Deleted:  len(deleted),
		},
	}
	return outputJSON(JSONEnvelope{
		Command: "log",
		Status:  "ok",
		Data:    data,
	})
}
