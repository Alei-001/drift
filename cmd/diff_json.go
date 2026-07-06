package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/porcelain"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/your-org/drift/internal/util/pathutil"
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
// envelope. It mirrors porcelain.DiffSnapshots but returns structured data
// instead of printing.
func diffSnapshotsJSON(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot, label1, label2 string) error {
	snap1Files := make(map[string]*core.FileEntry)
	for i := range snap1.Files {
		snap1Files[snap1.Files[i].Path] = &snap1.Files[i]
	}

	added := make([]string, 0)
	modified := make([]string, 0)
	deleted := make([]string, 0)

	for i := range snap2.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		e2 := &snap2.Files[i]
		e1, exists := snap1Files[e2.Path]
		if !exists {
			added = append(added, e2.Path)
			continue
		}
		if e1.Size != e2.Size || !slices.Equal(e1.Chunks, e2.Chunks) {
			modified = append(modified, e2.Path)
		}
		delete(snap1Files, e2.Path)
	}
	for path := range snap1Files {
		deleted = append(deleted, path)
	}

	return outputJSON(JSONEnvelope{
		Command: "diff",
		Status:  "ok",
		Data: diffFilesData{
			Base:     label1,
			Target:   label2,
			Mode:     "files",
			Added:    added,
			Modified: modified,
			Deleted:  deleted,
			Summary: diffFilesSummary{
				Total:    len(added) + len(modified) + len(deleted),
				Added:    len(added),
				Modified: len(modified),
				Deleted:  len(deleted),
			},
		},
	})
}

// diffWorkspaceJSON emits a file-level workspace-vs-snapshot diff as a JSON
// envelope. It walks the workspace via fsutil.Walk and classifies files by
// size and content hash, mirroring porcelain.DiffWorkspaceVsSnapshot.
func diffWorkspaceJSON(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot, snapLabel string) error {
	snapFiles := make(map[string]*core.FileEntry)
	for i := range snap.Files {
		snapFiles[snap.Files[i].Path] = &snap.Files[i]
	}

	added := make([]string, 0)
	modified := make([]string, 0)
	deleted := make([]string, 0)

	walkErr := fsutil.Walk(cwd, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := pathutil.Rel(cwd, path)
		if relErr != nil {
			return nil
		}
		snapEntry, exists := snapFiles[rel]
		if !exists {
			added = append(added, rel)
			return nil
		}
		if info.Size() != snapEntry.Size {
			modified = append(modified, rel)
		} else {
			workHash, hashErr := porcelain.ComputeFileHash(path, cfg)
			if hashErr != nil || workHash != snapEntry.Hash {
				modified = append(modified, rel)
			}
		}
		delete(snapFiles, rel)
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk workspace: %w", walkErr)
	}
	for path := range snapFiles {
		deleted = append(deleted, path)
	}

	return outputJSON(JSONEnvelope{
		Command: "diff",
		Status:  "ok",
		Data: diffFilesData{
			Base:     snapLabel,
			Target:   "workspace",
			Mode:     "files",
			Added:    added,
			Modified: modified,
			Deleted:  deleted,
			Summary: diffFilesSummary{
				Total:    len(added) + len(modified) + len(deleted),
				Added:    len(added),
				Modified: len(modified),
				Deleted:  len(deleted),
			},
		},
	})
}

// diffStatSnapshotsJSON emits a --stat diff between two snapshots as JSON.
func diffStatSnapshotsJSON(ctx context.Context, store storage.Storer, snap1, snap2 *core.Snapshot, label1, label2 string) error {
	stats, err := computeStatSnapshots(ctx, store, snap1, snap2)
	if err != nil {
		reportFailed("Diff", "diff", err.Error(), "")
		return ErrSilent
	}
	return outputStatJSON(label1, label2, stats)
}

// diffStatWorkspaceJSON emits a --stat workspace-vs-snapshot diff as JSON.
func diffStatWorkspaceJSON(ctx context.Context, store storage.Storer, cwd string, cfg *core.CoreConfig, snap *core.Snapshot, snapLabel string) error {
	stats, err := computeStatWorkspace(ctx, store, cwd, cfg, snap)
	if err != nil {
		reportFailed("Diff", "diff", err.Error(), "")
		return ErrSilent
	}
	return outputStatJSON(snapLabel, "workspace", stats)
}

// outputStatJSON converts fileStat results into a JSON envelope. OldSize and
// NewSize are only included for binary files, matching the schema where text
// files show insertions/deletions and binary files show sizes.
func outputStatJSON(base, target string, stats []fileStat) error {
	files := make([]diffStatFile, 0, len(stats))
	totalIns, totalDel := 0, 0
	for _, s := range stats {
		entry := diffStatFile{
			Path:       s.path,
			Insertions: s.insertions,
			Deletions:  s.deletions,
			Binary:     s.binary,
		}
		if s.binary {
			entry.OldSize = s.oldSize
			entry.NewSize = s.newSize
		}
		files = append(files, entry)
		totalIns += s.insertions
		totalDel += s.deletions
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
