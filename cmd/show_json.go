package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/porcelain"
	"github.com/Alei-001/drift/internal/storage"
)

type showFileListEntry struct {
	Path       string `json:"path"`
	Size       int64  `json:"size"`
	Type       string `json:"type"`
	Dimensions string `json:"dimensions,omitempty"`
}

type showFileListData struct {
	Version string              `json:"version"`
	Files   []showFileListEntry `json:"files"`
}

type showTextFileData struct {
	Version string `json:"version"`
	File    string `json:"file"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type showBinaryFileData struct {
	Version    string `json:"version"`
	File       string `json:"file"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Dimensions string `json:"dimensions,omitempty"`
	Modified   string `json:"modified,omitempty"`
}

type showOpenData struct {
	Opened  bool   `json:"opened"`
	Version string `json:"version"`
	File    string `json:"file"`
}

func showFileListJSON(ctx context.Context, store storage.Storer, snap *core.Snapshot, versionLabel string) error {
	files := make([]showFileListEntry, 0, len(snap.Files))
	for i := range snap.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		f := &snap.Files[i]
		typeLabel := porcelain.DetectFileTypeLabel(ctx, store, f)
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

func showFileJSON(ctx context.Context, store storage.Storer, cwd string, snapshot *core.Snapshot, versionLabel, filePath string) error {
	result, err := porcelain.ReadSnapshotFile(ctx, store, snapshot, cwd, filePath)
	if err != nil {
		if errors.Is(err, porcelain.ErrFileNotFound) {
			reportFailed("Show", "show", fmt.Sprintf("'%s' not found in snapshot %s.", filePath, versionLabel),
				fmt.Sprintf("use 'drift show %s' to list files in this snapshot.", versionLabel), err)
			return ErrSilent
		}
		if errors.Is(err, porcelain.ErrInvalidPath) {
			reportFailed("Show", "show", fmt.Sprintf("cannot resolve path '%s'.", filePath),
				"use a relative path from the project root.", err)
			return ErrSilent
		}
		reportFailed("Show", "show", fmt.Sprintf("cannot read '%s' from snapshot: %s.", filePath, err),
			"the chunk data may be missing or corrupted; use 'drift check' to verify.", err)
		return ErrSilent
	}

	if showOpen {
		if err := openExternal(versionLabel, filePath, bytes.NewReader(result.Content)); err != nil {
			return ErrSilent
		}
		return outputJSON(JSONEnvelope{
			Command: "show",
			Status:  "ok",
			Data:    showOpenData{Opened: true, Version: versionLabel, File: filePath},
		})
	}

	if result.Kind == "text" {
		return outputJSON(JSONEnvelope{
			Command: "show",
			Status:  "ok",
			Data: showTextFileData{
				Version: versionLabel,
				File:    filePath,
				Type:    "text",
				Content: string(result.Content),
			},
		})
	}

	binaryData := showBinaryFileData{
		Version: versionLabel,
		File:    filePath,
		Size:    result.Size,
	}
	if result.Kind == "image" {
		binaryData.Type = "image"
		if result.Dimensions != "" {
			binaryData.Dimensions = result.Dimensions
		}
	} else {
		binaryData.Type = "binary"
	}
	if result.ModTime > 0 {
		binaryData.Modified = time.Unix(0, result.ModTime).Format("01-02 15:04")
	}
	return outputJSON(JSONEnvelope{
		Command: "show",
		Status:  "ok",
		Data:    binaryData,
		Hint:    hintPtr("use --open to view with system program."),
	})
}
