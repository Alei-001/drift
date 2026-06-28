package storage

import "github.com/your-org/drift/core"

// ChunkStorer provides access to chunk storage.
type ChunkStorer interface {
	HasChunk(hash core.Hash) bool
	GetChunk(hash core.Hash) (*core.Chunk, error)
	PutChunk(chunk *core.Chunk) error
}
