package store

// Chunk wire format constants shared between filesystem backend and
// remote sync. The on-disk chunk file and the remote chunk object use
// the same format: a 1-byte header followed by (optionally compressed)
// chunk data. Bit 0 of the header indicates zstd compression.
//
// Keeping these constants in the storage interface package (rather than
// duplicated in filesystem and remote) eliminates the dedup violation
// and ensures both sides agree on the wire format.
const (
	ChunkHeaderSize          = 1
	ChunkFlagCompressed byte = 0x01
	// ChunkFlagEncrypted is reserved for future end-to-end encryption of
	// remote-synced chunks. Not yet implemented.
	ChunkFlagEncrypted byte = 0x02
)
