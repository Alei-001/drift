package engine

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestDetectTextFileByExtension(t *testing.T) {
	tests := []string{
		"main.go",
		"README.md",
		"script.sh",
		"config.json",
		"style.css",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			engine := DetectEngine(path, []byte("test data"))
			if engine == nil {
				t.Fatalf("expected engine for %s, got nil", path)
			}
			if engine.Name() != "text" {
				t.Errorf("expected 'text' engine for %s, got %q", path, engine.Name())
			}
		})
	}
}

func TestDetectTextFileByBaseName(t *testing.T) {
	tests := []string{
		"Makefile",
		"Dockerfile",
		".gitignore",
		"LICENSE",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			engine := DetectEngine(path, []byte("some content"))
			if engine == nil {
				t.Fatalf("expected engine for %s, got nil", path)
			}
			if engine.Name() != "text" {
				t.Errorf("expected 'text' engine for %s, got %q", path, engine.Name())
			}
		})
	}
}

func TestDetectTextFileNoNullBytes(t *testing.T) {
	// .dat extension is not in textExtensions, but content has no null bytes
	engine := DetectEngine("data.dat", []byte("hello world\nsome text\n"))
	if engine == nil {
		t.Fatal("expected engine for text content, got nil")
	}
	if engine.Name() != "text" {
		t.Errorf("expected 'text' engine, got %q", engine.Name())
	}
}

func TestDetectBinaryFile(t *testing.T) {
	// .dat with null bytes in header should fall through text to binary
	data := []byte{0x00, 0x01, 0x02, 0x00, 0x04}
	engine := DetectEngine("data.dat", data)
	if engine == nil {
		t.Fatal("expected engine for binary data, got nil")
	}
	if engine.Name() != "binary" {
		t.Errorf("expected 'binary' engine, got %q", engine.Name())
	}
}

func TestDetectBinaryFileWithNullBytes(t *testing.T) {
	// Any file with null bytes should be binary
	data := append([]byte("hello"), 0x00, 0x01)
	engine := DetectEngine("output.bin", data)
	if engine == nil {
		t.Fatal("expected binary engine for data with null bytes, got nil")
	}
	if engine.Name() != "binary" {
		t.Errorf("expected 'binary' engine, got %q", engine.Name())
	}
}

func TestDetectEmptyFile(t *testing.T) {
	// Empty file without known extension should still match binary (fallback)
	engine := DetectEngine("unknown.xyz", []byte{})
	if engine == nil {
		t.Fatal("expected binary engine as fallback, got nil")
	}
	if engine.Name() != "binary" {
		t.Errorf("expected 'binary' engine, got %q", engine.Name())
	}
}

func TestBinaryEnginePreview(t *testing.T) {
	engine := DetectEngine("data.bin", []byte{0x00, 0x01, 0x02})
	preview, err := engine.Preview([]byte{0x00, 0x01, 0x02}, 3, nil, 10)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	if preview != "[binary file]" {
		t.Errorf("expected '[binary file]', got %q", preview)
	}
}

func TestTextEnginePreview(t *testing.T) {
	engine := DetectEngine("script.sh", []byte("#!/bin/sh"))
	data := []byte("line1\nline2\nline3\nline4\nline5")
	preview, err := engine.Preview(nil, int64(len(data)), bytes.NewReader(data), 2)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	expected := "line1\nline2"
	if preview != expected {
		t.Errorf("expected %q, got %q", expected, preview)
	}
}

func TestTextEnginePreviewDefaultLines(t *testing.T) {
	engine := DetectEngine("README.md", []byte("text"))
	lines := make([]string, 25)
	for i := range lines {
		lines[i] = "line"
	}
	data := []byte(strings.Join(lines, "\n"))
	preview, err := engine.Preview(nil, int64(len(data)), bytes.NewReader(data), 0)
	if err != nil {
		t.Fatalf("Preview failed: %v", err)
	}
	// Should default to 20 lines
	parts := strings.Split(preview, "\n")
	if len(parts) != 20 {
		t.Errorf("expected 20 lines with maxLines=0, got %d", len(parts))
	}
}

func TestBinaryDiff(t *testing.T) {
	engine := DetectEngine("a.bin", []byte{0x00, 0x01})
	diff, err := engine.Diff(context.Background(), "old.bin", bytes.NewReader([]byte{0x00, 0x01}), "new.bin", bytes.NewReader([]byte{0x00, 0x02}))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "binary files differ" {
		t.Errorf("expected 'binary files differ', got %q", diff)
	}

	// Same data should produce empty diff
	diff, err = engine.Diff(context.Background(), "old.bin", bytes.NewReader([]byte{0x00, 0x01}), "new.bin", bytes.NewReader([]byte{0x00, 0x01}))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for identical data, got %q", diff)
	}
}

func TestTextDiff(t *testing.T) {
	engine := DetectEngine("file.txt", []byte("text"))

	oldData := []byte("line1\nline2\nline3\n")
	newData := []byte("line1\nline2_modified\nline3\n")
	diff, err := engine.Diff(context.Background(), "old.txt", bytes.NewReader(oldData), "new.txt", bytes.NewReader(newData))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(diff, "--- old.txt") {
		t.Error("diff should contain '--- old.txt'")
	}
	if !strings.Contains(diff, "+++ new.txt") {
		t.Error("diff should contain '+++ new.txt'")
	}
	if !strings.Contains(diff, "-line2") {
		t.Error("diff should contain '-line2'")
	}
	if !strings.Contains(diff, "+line2_modified") {
		t.Error("diff should contain '+line2_modified'")
	}
}

func TestTextDiffIdentical(t *testing.T) {
	engine := DetectEngine("file.txt", []byte("text"))

	data := []byte("line1\nline2\nline3\n")
	diff, err := engine.Diff(context.Background(), "old.txt", bytes.NewReader(data), "new.txt", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for identical data, got %q", diff)
	}
}

func TestTextDiff_CRLF(t *testing.T) {
	engine := DetectEngine("file.txt", []byte("text"))
	oldData := []byte("line1\r\nline2\r\nline3\r\n")
	newData := []byte("line1\nline2\nline3\n")
	diff, err := engine.Diff(context.Background(), "old.txt", bytes.NewReader(oldData), "new.txt", bytes.NewReader(newData))
	if err != nil {
		t.Fatal(err)
	}
	if diff != "" {
		t.Errorf("expected no diff for CRLF vs LF, got:\n%s", diff)
	}
}

func TestTextDiff_HunkMerge(t *testing.T) {
	engine := DetectEngine("file.txt", []byte("text"))
	oldData := []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\n")
	newData := []byte("line1\nCHANGED2\nline3\nline4\nline5\nline6\nCHANGED7\nline8\n")
	diff, err := engine.Diff(context.Background(), "old.txt", bytes.NewReader(oldData), "new.txt", bytes.NewReader(newData))
	if err != nil {
		t.Fatal(err)
	}
	// Count @@ markers — should be 2 (opening @@ ... @@), meaning 1 merged hunk
	atAtCount := strings.Count(diff, "@@")
	if atAtCount != 2 {
		t.Errorf("expected 1 hunk (2 @@ markers), got %d @@ markers:\n%s", atAtCount, diff)
	}
}

func TestTextDiff_LargeFile(t *testing.T) {
	engine := DetectEngine("file.txt", []byte("text"))
	var oldLines, newLines []string
	for i := 0; i < 10000; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line %d", i))
		newLines = append(newLines, fmt.Sprintf("line %d", i))
	}
	newLines[5000] = "CHANGED"
	oldData := []byte(strings.Join(oldLines, "\n"))
	newData := []byte(strings.Join(newLines, "\n"))
	_, err := engine.Diff(context.Background(), "old.txt", bytes.NewReader(oldData), "new.txt", bytes.NewReader(newData))
	if err != nil {
		t.Fatal(err)
	}
}
