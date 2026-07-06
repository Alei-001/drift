package text

import (
	"bytes"
	"path/filepath"
	"strings"
)

var textExtensions = map[string]bool{
	".txt": true, ".md": true, ".go": true, ".rs": true, ".js": true, ".ts": true,
	".py": true, ".java": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true,
	".html": true, ".css": true, ".json": true, ".xml": true, ".yaml": true,
	".yml": true, ".toml": true, ".ini": true, ".cfg": true, ".conf": true,
	".sh": true, ".bat": true, ".ps1": true, ".rb": true, ".php": true,
	".swift": true, ".kt": true, ".scala": true, ".r": true, ".sql": true,
	".csv": true, ".log": true, ".svg": true, ".tex": true,
}

var textBasenames = map[string]bool{
	"Makefile": true, "Dockerfile": true, "LICENSE": true, "README": true,
	".gitignore": true, ".gitattributes": true, ".editorconfig": true,
	".env": true, ".dockerignore": true,
}

// TextEngine handles text files.
type TextEngine struct{}

// NewEngine creates a new TextEngine.
func NewEngine() *TextEngine {
	return &TextEngine{}
}

// Name returns "text".
func (e *TextEngine) Name() string {
	return "text"
}

// DetectByMagic checks file header signatures. Text has no unified magic
// byte signature, so this always returns false.
func (e *TextEngine) DetectByMagic(header []byte) bool {
	return false
}

// DetectByExtension checks if the path's extension or basename is a known
// text type.
func (e *TextEngine) DetectByExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)
	return textExtensions[ext] || textBasenames[base]
}

// DetectByHeuristic is the last-resort content sniffing: a header without
// null bytes is treated as text. Used for extensionless or unknown-extension
// files. Before applying the null-byte heuristic, it checks against known
// image/video magic bytes to avoid misclassifying binary files as text.
func (e *TextEngine) DetectByHeuristic(path string, header []byte) bool {
	if len(header) == 0 {
		return false
	}
	if matchesBinaryMagic(header) {
		return false
	}
	return !bytes.Contains(header, []byte{0})
}

// matchesBinaryMagic checks if the header matches known image or video
// magic byte prefixes. This prevents the text engine from claiming binary
// files (e.g. BMP, TIFF) that happen to have no null bytes in their header.
func matchesBinaryMagic(header []byte) bool {
	// PNG: \x89PNG
	if len(header) >= 4 && header[0] == 0x89 && header[1] == 'P' && header[2] == 'N' && header[3] == 'G' {
		return true
	}
	// JPEG: \xFF\xD8\xFF
	if len(header) >= 3 && header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return true
	}
	// GIF: GIF8
	if len(header) >= 4 && string(header[:4]) == "GIF8" {
		return true
	}
	// WebP / AVI: RIFF container
	if len(header) >= 4 && string(header[:4]) == "RIFF" {
		return true
	}
	// BMP: BM
	if len(header) >= 2 && header[0] == 'B' && header[1] == 'M' {
		return true
	}
	// TIFF: II*\x00 (LE) or MM\x00* (BE)
	if len(header) >= 4 {
		if header[0] == 'I' && header[1] == 'I' && header[2] == 0x2A && header[3] == 0x00 {
			return true
		}
		if header[0] == 'M' && header[1] == 'M' && header[2] == 0x00 && header[3] == 0x2A {
			return true
		}
	}
	// MKV/WebM: EBML header
	if len(header) >= 4 && header[0] == 0x1A && header[1] == 0x45 && header[2] == 0xDF && header[3] == 0xA3 {
		return true
	}
	// MP4/MOV: ftyp box at offset 4
	if len(header) >= 8 && string(header[4:8]) == "ftyp" {
		return true
	}
	return false
}
