package binary

import "github.com/your-org/drift/internal/core"

// Metadata returns the file metadata for binary files.
func (e *BinaryEngine) Metadata() *core.FileMetadata {
	return &core.FileMetadata{MIMEType: "application/octet-stream"}
}
