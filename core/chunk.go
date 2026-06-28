package core

// ChunkFlag represents flags for a chunk.
type ChunkFlag uint8

const (
	ChunkFlagNone       ChunkFlag = 0
	ChunkFlagCompressed ChunkFlag = 1
)

// Chunk represents a content-addressed chunk of data.
type Chunk struct {
	Hash  Hash
	Size  uint32
	Data  []byte
	Flags ChunkFlag
}
