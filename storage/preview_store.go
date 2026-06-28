package storage

import "github.com/your-org/drift/core"

// PreviewStorer provides access to preview (thumbnail) data.
type PreviewStorer interface {
	GetPreview(hash core.Hash, size int) ([]byte, error)
	PutPreview(hash core.Hash, size int, data []byte) error
}
