package text

import "github.com/your-org/drift/internal/core"

// Metadata returns the file metadata for text files.
func (e *TextEngine) Metadata() *core.FileMetadata {
	return &core.FileMetadata{MIMEType: "text/plain"}
}
