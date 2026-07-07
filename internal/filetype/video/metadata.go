package video

import "github.com/Alei-001/drift/internal/core"

// Metadata returns the file metadata for video files. The MIME type is a
// generic octet-stream placeholder; per-container refinement (video/mp4,
// etc.) is a future enhancement tracked separately.
func (e *VideoEngine) Metadata() *core.FileMetadata {
	return &core.FileMetadata{MIMEType: "application/octet-stream"}
}
