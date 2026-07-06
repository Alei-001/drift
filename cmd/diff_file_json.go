package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/pathutil"
)

// diffFileJSON emits a single-file diff between two snapshots as a JSON
// envelope. It resolves the file in each snapshot, then streams both versions
// to produce a text diff or binary metadata, mirroring
// porcelain.DiffFileInSnapshots.
func diffFileJSON(ctx context.Context, store storage.Storer, cwd string, snap1, snap2 *core.Snapshot, label1, label2, filePath string) error {
	normalizedPath, err := pathutil.RelToWorkDir(cwd, filePath)
	if err != nil {
		reportFailed("Diff", "diff", fmt.Sprintf("cannot resolve path '%s'.", filePath),
			"use a relative path from the project root.")
		return ErrSilent
	}

	var entry1, entry2 *core.FileEntry
	for i := range snap1.Files {
		if snap1.Files[i].Path == normalizedPath {
			entry1 = &snap1.Files[i]
			break
		}
	}
	for i := range snap2.Files {
		if snap2.Files[i].Path == normalizedPath {
			entry2 = &snap2.Files[i]
			break
		}
	}

	if entry1 == nil && entry2 != nil {
		return outputJSON(JSONEnvelope{
			Command: "diff", Status: "ok",
			Data: diffFileData{Base: label1, Target: label2, Mode: "file", File: filePath, Type: "added", NewSize: entry2.Size},
		})
	}
	if entry1 != nil && entry2 == nil {
		return outputJSON(JSONEnvelope{
			Command: "diff", Status: "ok",
			Data: diffFileData{Base: label1, Target: label2, Mode: "file", File: filePath, Type: "deleted", OldSize: entry1.Size},
		})
	}
	if entry1 == nil && entry2 == nil {
		reportFailed("Diff", "diff", fmt.Sprintf("'%s' not found in either snapshot.", filePath), "")
		return ErrSilent
	}

	if entry1.Size == entry2.Size && slices.Equal(entry1.Chunks, entry2.Chunks) {
		return outputJSON(JSONEnvelope{
			Command: "diff", Status: "ok",
			Data: diffFileData{Base: label1, Target: label2, Mode: "file", File: filePath, Type: "unchanged"},
		})
	}

	reader1 := stream.NewChunkReader(ctx, store, entry1.Chunks)
	reader2 := stream.NewChunkReader(ctx, store, entry2.Chunks)
	header1, fullReader1, err := stream.PeekHeader(reader1, core.HeaderPeekSize)
	if err != nil {
		reportFailed("Diff", "diff", fmt.Sprintf("cannot read chunk for %s: %s.", filePath, err), "")
		return ErrSilent
	}
	header2, fullReader2, err := stream.PeekHeader(reader2, core.HeaderPeekSize)
	if err != nil {
		reportFailed("Diff", "diff", fmt.Sprintf("cannot read chunk for %s: %s.", filePath, err), "")
		return ErrSilent
	}

	engine := filetype.DetectEngine(normalizedPath, header2)
	if engine != nil && engine.Name() == "text" {
		diff, diffErr := engine.Diff(ctx, label1+"/"+filePath, fullReader1, label2+"/"+filePath, fullReader2)
		if diffErr != nil {
			reportFailed("Diff", "diff", fmt.Sprintf("cannot diff %s: %s.", filePath, diffErr), "")
			return ErrSilent
		}
		return outputJSON(JSONEnvelope{
			Command: "diff", Status: "ok",
			Data: diffFileData{Base: label1, Target: label2, Mode: "file", File: filePath, Type: "text", Diff: diff},
		})
	}

	data := diffFileData{
		Base: label1, Target: label2, Mode: "file", File: filePath, Type: "binary",
		OldSize: entry1.Size, NewSize: entry2.Size,
	}
	if dims := imageDimensions(header1); dims != "" {
		data.OldDimensions = dims
	}
	if dims := imageDimensions(header2); dims != "" {
		data.NewDimensions = dims
	}
	return outputJSON(JSONEnvelope{Command: "diff", Status: "ok", Data: data})
}

// diffWorkspaceFileJSON emits a single-file workspace-vs-snapshot diff as a
// JSON envelope. It opens the workspace file, streams the snapshot version,
// and produces a text diff or binary metadata, mirroring
// porcelain.DiffWorkspaceFileVsSnapshot.
func diffWorkspaceFileJSON(ctx context.Context, store storage.Storer, cwd string, snap *core.Snapshot, snapLabel, filePath string) error {
	normalizedPath, err := pathutil.RelToWorkDir(cwd, filePath)
	if err != nil {
		reportFailed("Diff", "diff", fmt.Sprintf("cannot resolve path '%s'.", filePath),
			"use a relative path from the project root.")
		return ErrSilent
	}

	var snapEntry *core.FileEntry
	for i := range snap.Files {
		if snap.Files[i].Path == normalizedPath {
			snapEntry = &snap.Files[i]
			break
		}
	}

	fullPath := filepath.Join(cwd, normalizedPath)
	info, statErr := os.Stat(fullPath)

	if snapEntry == nil {
		if os.IsNotExist(statErr) {
			reportFailed("Diff", "diff", fmt.Sprintf("'%s' not found in snapshot or workspace.", filePath), "")
			return ErrSilent
		}
		if statErr != nil {
			reportFailed("Diff", "diff", fmt.Sprintf("cannot stat workspace file %s: %s", filePath, statErr), "")
			return ErrSilent
		}
		return outputJSON(JSONEnvelope{
			Command: "diff", Status: "ok",
			Data: diffFileData{Base: snapLabel, Target: "workspace", Mode: "file", File: filePath, Type: "added", NewSize: info.Size()},
		})
	}
	if os.IsNotExist(statErr) {
		return outputJSON(JSONEnvelope{
			Command: "diff", Status: "ok",
			Data: diffFileData{Base: snapLabel, Target: "workspace", Mode: "file", File: filePath, Type: "deleted", OldSize: snapEntry.Size},
		})
	}

	if info.Size() == snapEntry.Size {
		workHash, hashErr := stream.HashFileContent(fullPath)
		if hashErr != nil {
			reportFailed("Diff", "diff", fmt.Sprintf("cannot hash %s: %s.", filePath, hashErr), "")
			return ErrSilent
		}
		snapHash, hashErr := stream.HashChunkData(ctx, store, snapEntry.Chunks)
		if hashErr != nil {
			reportFailed("Diff", "diff", fmt.Sprintf("cannot hash snapshot chunks for %s: %s.", filePath, hashErr), "")
			return ErrSilent
		}
		if workHash == snapHash {
			return outputJSON(JSONEnvelope{
				Command: "diff", Status: "ok",
				Data: diffFileData{Base: snapLabel, Target: "workspace", Mode: "file", File: filePath, Type: "unchanged"},
			})
		}
	}

	workFile, err := os.Open(fullPath)
	if err != nil {
		reportFailed("Diff", "diff", fmt.Sprintf("cannot open %s: %s.", fullPath, err), "")
		return ErrSilent
	}
	defer workFile.Close()

	header, workReader, err := stream.PeekHeader(workFile, core.HeaderPeekSize)
	if err != nil {
		reportFailed("Diff", "diff", fmt.Sprintf("cannot read header %s: %s.", fullPath, err), "")
		return ErrSilent
	}
	engine := filetype.DetectEngine(normalizedPath, header)
	snapReader := stream.NewChunkReader(ctx, store, snapEntry.Chunks)

	if engine != nil && engine.Name() == "text" {
		diff, diffErr := engine.Diff(ctx, snapLabel+"/"+filePath, snapReader, "workspace/"+filePath, workReader)
		if diffErr != nil {
			reportFailed("Diff", "diff", fmt.Sprintf("cannot diff %s: %s.", filePath, diffErr), "")
			return ErrSilent
		}
		return outputJSON(JSONEnvelope{
			Command: "diff", Status: "ok",
			Data: diffFileData{Base: snapLabel, Target: "workspace", Mode: "file", File: filePath, Type: "text", Diff: diff},
		})
	}

	snapHeader, _, err := stream.PeekHeader(snapReader, core.HeaderPeekSize)
	if err != nil {
		reportFailed("Diff", "diff", fmt.Sprintf("cannot read snapshot header %s: %s.", filePath, err), "")
		return ErrSilent
	}
	data := diffFileData{
		Base: snapLabel, Target: "workspace", Mode: "file", File: filePath, Type: "binary",
		OldSize: snapEntry.Size, NewSize: info.Size(),
	}
	if dims := imageDimensions(snapHeader); dims != "" {
		data.OldDimensions = dims
	}
	if dims := imageDimensions(header); dims != "" {
		data.NewDimensions = dims
	}
	return outputJSON(JSONEnvelope{Command: "diff", Status: "ok", Data: data})
}
