package cmd

import (
	"context"
	"strings"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
)

// diffFileJSON emits a single-file diff between two snapshots as a JSON
// envelope. The content-level diff is delegated to
// porcelain.DiffFileInSnapshots; this function only wraps the structured
// result in the JSON envelope.
func diffFileJSON(ctx context.Context, store storage.Storer, cwd string, snap1, snap2 *core.Snapshot, label1, label2, filePath string) error {
	result := porcelain.DiffFileInSnapshots(ctx, store, cwd, snap1, snap2, filePath)
	if result.Stderr != "" {
		reportFailed("Diff", "diff", strings.TrimSpace(result.Stderr), "")
		return ErrSilent
	}
	return outputJSON(JSONEnvelope{
		Command: "diff", Status: "ok",
		Data: diffFileData{
			Base:          label1,
			Target:        label2,
			Mode:          "file",
			File:          filePath,
			Type:          result.Kind,
			Diff:          result.Diff,
			OldSize:       result.OldSize,
			NewSize:       result.NewSize,
			OldDimensions: result.OldDimensions,
			NewDimensions: result.NewDimensions,
		},
	})
}

// diffWorkspaceFileJSON emits a single-file workspace-vs-snapshot diff as a
// JSON envelope. The content-level diff is delegated to
// porcelain.DiffWorkspaceFileVsSnapshot; this function only wraps the
// structured result in the JSON envelope.
func diffWorkspaceFileJSON(ctx context.Context, store storage.Storer, cwd string, snap *core.Snapshot, snapLabel, filePath string) error {
	result, err := porcelain.DiffWorkspaceFileVsSnapshot(ctx, store, cwd, snap, filePath)
	if err != nil {
		reportFailed("Diff", "diff", err.Error(), "")
		return ErrSilent
	}
	if result.Stderr != "" {
		reportFailed("Diff", "diff", strings.TrimSpace(result.Stderr), "")
		return ErrSilent
	}
	return outputJSON(JSONEnvelope{
		Command: "diff", Status: "ok",
		Data: diffFileData{
			Base:          snapLabel,
			Target:        "workspace",
			Mode:          "file",
			File:          filePath,
			Type:          result.Kind,
			Diff:          result.Diff,
			OldSize:       result.OldSize,
			NewSize:       result.NewSize,
			OldDimensions: result.OldDimensions,
			NewDimensions: result.NewDimensions,
		},
	})
}
