package image

import "github.com/Alei-001/drift/internal/core"

// Metadata returns the file metadata for image files. The MIME type is a
// generic octet-stream placeholder; per-format refinement (image/png, etc.)
// is a future enhancement tracked separately.
func (e *ImageEngine) Metadata() *core.FileMetadata {
	return &core.FileMetadata{MIMEType: "application/octet-stream"}
}
