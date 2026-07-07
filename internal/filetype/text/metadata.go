package text

import "github.com/Alei-001/drift/internal/core"

// Metadata returns the file metadata for text files.
func (e *TextEngine) Metadata() *core.FileMetadata {
	return &core.FileMetadata{MIMEType: "text/plain"}
}
