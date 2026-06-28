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

// Detect checks if a file is a text file by extension or by content.
func (e *TextEngine) Detect(path string, header []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)

	if textExtensions[ext] || textBasenames[base] {
		return true
	}

	if len(header) == 0 {
		return false
	}

	return !bytes.Contains(header, []byte{0})
}
