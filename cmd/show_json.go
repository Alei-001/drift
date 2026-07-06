package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/storage/stream"
	"github.com/your-org/drift/internal/util/pathutil"
)

// showFileListEntry is one file row in the JSON output of `drift show <version>`.
// Dimensions is only set for image files; other types omit it.
type showFileListEntry struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Type       string `json:"type"`
	Dimensions string `json:"dimensions,omitempty"`
}

// showFileListData is the data payload for `drift show <version> --json`.
type showFileListData struct {
	Version string              `json:"version"`
	Files   []showFileListEntry `json:"files"`
}

// showTextFileData is the data payload for `drift show <version> <file> --json`
// when the file is text.
type showTextFileData struct {
	Version string `json:"version"`
	File    string `json:"file"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

// showBinaryFileData is the data payload for `drift show <version> <file> --json`
// when the file is binary or image. Dimensions and Modified are omitted when
// not applicable.
type showBinaryFileData struct {
	Version    string `json:"version"`
	File       string `json:"file"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Dimensions string `json:"dimensions,omitempty"`
	Modified   string `json:"modified,omitempty"`
}

// showOpenData is the data payload for `drift show --open --json`.
type showOpenData struct {
	Opened  bool   `json:"opened"`
	Version string `json:"version"`
	File    string `json:"file"`
}

// showFileListJSON emits the file listing of a snapshot as a JSON envelope.
// It reuses fileTypeLabel to determine the type, splitting the "image (WxH)"
// format into separate type and dimensions fields for structured output.
func showFileListJSON(ctx context.Context, store storage.Storer, snap *core.Snapshot, versionLabel string) error {
	files := make([]showFileListEntry, 0, len(snap.Files))
	for i := range snap.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := &snap.Files[i]
		typeLabel := fileTypeLabel(ctx, store, f)
		entry := showFileListEntry{Path: f.Path, Size: f.Size}
		if strings.HasPrefix(typeLabel, "image (") && strings.HasSuffix(typeLabel, ")") {
			entry.Type = "image"
			entry.Dimensions = typeLabel[len("image (") : len(typeLabel)-1]
		} else {
			entry.Type = typeLabel
		}
		files = append(files, entry)
	}
	return outputJSON(JSONEnvelope{
		Command: "show",
		Status:  "ok",
		Data: showFileListData{
			Version: versionLabel,
			Files:   files,
		},
	})
}

// showFileJSON handles all single-file JSON output paths: text content,
// binary/image metadata, and --open. It performs its own chunk reading so
// the human-readable path in showFile is bypassed entirely.
func showFileJSON(ctx context.Context, store storage.Storer, cwd string, snapshot *core.Snapshot, versionLabel, filePath string) error {
	normalizedPath, err := pathutil.RelToWorkDir(cwd, filePath)
	if err != nil {
		reportFailed("Show", "show", fmt.Sprintf("cannot resolve path '%s'.", filePath),
			"use a relative path from the project root.")
		return ErrSilent
	}

	var targetEntry *core.FileEntry
	for i := range snapshot.Files {
		if snapshot.Files[i].Path == normalizedPath {
			targetEntry = &snapshot.Files[i]
			break
		}
	}
	if targetEntry == nil {
		reportFailed("Show", "show", fmt.Sprintf("'%s' not found in snapshot %s.", filePath, versionLabel),
			fmt.Sprintf("use 'drift show %s' to list files in this snapshot.", versionLabel))
		return ErrSilent
	}

	chunkR := stream.NewChunkReader(ctx, store, targetEntry.Chunks)
	header, fullReader, err := stream.PeekHeader(chunkR, core.HeaderPeekSize)
	if err != nil {
		reportFailed("Show", "show", fmt.Sprintf("cannot read '%s' from snapshot: %s.", filePath, err),
			"the chunk data may be missing or corrupted; use 'drift check' to verify.")
		return ErrSilent
	}
	engine := filetype.DetectEngine(normalizedPath, header)

	if showOpen {
		if err := openExternal(versionLabel, filePath, fullReader); err != nil {
			return ErrSilent
		}
		return outputJSON(JSONEnvelope{
			Command: "show",
			Status:  "ok",
			Data:    showOpenData{Opened: true, Version: versionLabel, File: filePath},
		})
	}

	if engine != nil && engine.Name() == "text" {
		data, err := io.ReadAll(fullReader)
		if err != nil {
			reportFailed("Show", "show", fmt.Sprintf("failed to read '%s': %s.", filePath, err), "")
			return ErrSilent
		}
		return outputJSON(JSONEnvelope{
			Command: "show",
			Status:  "ok",
			Data: showTextFileData{
				Version: versionLabel,
				File:    filePath,
				Type:    "text",
				Content: string(data),
			},
		})
	}

	// Binary or image file: emit metadata.
	binaryData := showBinaryFileData{
		Version: versionLabel,
		File:    filePath,
		Size:    targetEntry.Size,
	}
	if engine != nil && engine.Name() == "image" {
		binaryData.Type = "image"
		if dims := imageDimensions(header); dims != "" {
			binaryData.Dimensions = dims
		}
	} else {
		binaryData.Type = "binary"
	}
	if targetEntry.ModTime > 0 {
		binaryData.Modified = time.Unix(0, targetEntry.ModTime).Format("01-02 15:04")
	}
	return outputJSON(JSONEnvelope{
		Command: "show",
		Status:  "ok",
		Data:    binaryData,
		Hint:    hintPtr("use --open to view with system program."),
	})
}
