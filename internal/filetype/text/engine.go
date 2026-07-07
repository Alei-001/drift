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

// DetectByHeuristic is the last-resort content sniffing used for
// extensionless or unknown-extension files. A header is treated as text
// only when ALL of the following hold:
//   - it is non-empty,
//   - it does not start with a known image/video magic signature,
//   - it contains no NUL bytes (0x00),
//   - its control-byte ratio is at most 10%.
//
// Control bytes are 0x01-0x1F (excluding \t, \n, \r) and 0x7F (DEL).
// High bytes (0x80-0xFF) are NOT counted as control bytes because valid
// UTF-8 text legitimately contains them. The 10% threshold catches raw
// binary data that happens to omit 0x00 (e.g. byte sequences 1..255,
// which are ~11% control bytes) while allowing text with occasional
// control characters.
func (e *TextEngine) DetectByHeuristic(path string, header []byte) bool {
	if len(header) == 0 {
		return false
	}
	if matchesBinaryMagic(header) {
		return false
	}
	if bytes.Contains(header, []byte{0}) {
		return false
	}
	var control int
	for _, b := range header {
		if b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b < 0x20 || b == 0x7F {
			control++
		}
	}
	return control*100/len(header) <= 10
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
