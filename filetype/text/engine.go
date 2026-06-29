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
// files.
func (e *TextEngine) DetectByHeuristic(path string, header []byte) bool {
	if len(header) == 0 {
		return false
	}
	return !bytes.Contains(header, []byte{0})
}
