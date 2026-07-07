package cmd

import (
	"context"
	"fmt"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
)

// diffFilesSummary is the change tally for file-level diff JSON output.
type diffFilesSummary struct {
	Total    int `json:"total"`
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

// diffFilesData is the data payload for `drift diff --json` (file-level mode).
type diffFilesData struct {
	Base     string           `json:"base"`
	Target   string           `json:"target"`
	Mode     string           `json:"mode"`
	Added    []string         `json:"added"`
	Modified []string         `json:"modified"`
	Deleted  []string         `json:"deleted"`
	Summary  diffFilesSummary `json:"summary"`
}

// diffStatFile is one file row in the --stat JSON output. OldSize and
// NewSize are only set for binary files; text files omit them.
type diffStatFile struct {
	Path       string `json:"path"`
	Insertions int    `json:"insertions"`
	Deletions  int    `json:"deletions"`
	Binary     bool   `json:"binary"`
	OldSize    int64  `json:"old_size,omitempty"`
	NewSize    int64  `json:"new_size,omitempty"`
}

// diffStatSummary is the tally for --stat JSON output.
type diffStatSummary struct {
	FilesChanged int `json:"files_changed"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// diffStatData is the data payload for `drift diff --stat --json`.
type diffStatData struct {
	Base    string          `json:"base"`
	Target  string          `json:"target"`
	Mode    string          `json:"mode"`
	Files   []diffStatFile  `json:"files"`
	Summary diffStatSummary `json:"summary"`
}

// diffFileData is the data payload for single-file diff JSON output. Fields
// like Diff, OldSize, NewSize, and dimensions are only set for the relevant
// file type (text vs binary vs added/deleted).
type diffFileData struct {
	Base          string `json:"base"`
	Target        string `json:"target"`
	Mode          string `json:"mode"`
	File          string `json:"file"`
	Type          string `json:"type"`
	Diff          string `json:"diff,omitempty"`
	OldSize       int64  `json:"old_size,omitempty"`
	NewSize       int64  `json:"new_size,omitempty"`
	OldDimensions string `json:"old_dimensions,omitempty"`
	NewDimensions string `json:"new_dimensions,omitempty"`
}

// diffSnapshotsJSON emits a file-level diff between two snapshots as a JSON
// envelope. Classification is delegated to porcelain.DiffSnapshots; this
// function only wraps the result in the JSON envelope.
func diffSnapshotsJSON(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot, label1, label2 string) error {
	result := porcelain.DiffSnapshots(snap1, snap2)
	return outputJSON(JSONEnvelope{
		Command: "diff",
		Status:  "ok",
		Data: diffFilesData{
			Base:     label1,
			Target:   label2,
			Mode:     "files",
			Added:    result.Added,
			Modified: result.Modified,
			Deleted:  result.Deleted,
			Summary: diffFilesSummary{
				Total:    len(result.Added) + len(result.Modified) + len(result.Deleted),
				Added:    len(result.Added),
				Modified: len(result.Modified),
				Deleted:  len(result.Deleted),
			},
		},
	})
}

// diffWorkspaceJSON emits a file-level workspace-vs-snapshot diff as a JSON
// envelope. Classification is delegated to porcelain.DiffWorkspaceVsSnapshot;
// this function only wraps the result in the JSON envelope.
func diffWorkspaceJSON(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot, snapLabel string) error {
	result, err := porcelain.DiffWorkspaceVsSnapshot(ctx, cwd, snap, cfg)
	if err != nil {
		return fmt.Errorf("walk workspace: %w", err)
	}
	return outputJSON(JSONEnvelope{
		Command: "diff",
		Status:  "ok",
		Data: diffFilesData{
			Base:     snapLabel,
			Target:   "workspace",
			Mode:     "files",
			Added:    result.Added,
			Modified: result.Modified,
			Deleted:  result.Deleted,
			Summary: diffFilesSummary{
				Total:    len(result.Added) + len(result.Modified) + len(result.Deleted),
				Added:    len(result.Added),
				Modified: len(result.Modified),
				Deleted:  len(result.Deleted),
			},
		},
	})
}

// diffStatSnapshotsJSON emits a --stat diff between two snapshots as JSON.
// The per-file computation is delegated to porcelain.ComputeStatSnapshots.
func diffStatSnapshotsJSON(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot, label1, label2 string) error {
	stats, err := porcelain.ComputeStatSnapshots(ctx, store, snap1, snap2)
	if err != nil {
		reportFailed("Diff", "diff", err.Error(), "")
		return ErrSilent
	}
	return outputStatJSON(label1, label2, stats)
}

// diffStatWorkspaceJSON emits a --stat workspace-vs-snapshot diff as JSON.
// The per-file computation is delegated to porcelain.ComputeStatWorkspace.
func diffStatWorkspaceJSON(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot, snapLabel string) error {
	stats, err := porcelain.ComputeStatWorkspace(ctx, store, cwd, cfg, snap)
	if err != nil {
		reportFailed("Diff", "diff", err.Error(), "")
		return ErrSilent
	}
	return outputStatJSON(snapLabel, "workspace", stats)
}

// outputStatJSON converts FileStat results into a JSON envelope. OldSize and
// NewSize are only included for binary files, matching the schema where text
// files show insertions/deletions and binary files show sizes.
func outputStatJSON(base, target string, stats []porcelain.FileStat) error {
	files := make([]diffStatFile, 0, len(stats))
	totalIns, totalDel := 0, 0
	for _, s := range stats {
		entry := diffStatFile{
			Path:       s.Path,
			Insertions: s.Insertions,
			Deletions:  s.Deletions,
			Binary:     s.Binary,
		}
		if s.Binary {
			entry.OldSize = s.OldSize
			entry.NewSize = s.NewSize
		}
		files = append(files, entry)
		totalIns += s.Insertions
		totalDel += s.Deletions
	}
	return outputJSON(JSONEnvelope{
		Command: "diff",
		Status:  "ok",
		Data: diffStatData{
			Base:   base,
			Target: target,
			Mode:   "stat",
			Files:  files,
			Summary: diffStatSummary{
				FilesChanged: len(files),
				Insertions:   totalIns,
				Deletions:    totalDel,
			},
		},
	})
}
