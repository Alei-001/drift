package core

import (
	"bytes"
	"sync"
)

// Buffer pool for short-lived bytes.Buffer allocations in hot paths
// (eol.go writers, add.go putBlobForAdd, worktree_helpers.go, etc.).
// Mirroring go-git's utils/sync/bytes.go pattern.

var bytesBufferPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// GetBuffer returns a *bytes.Buffer from the pool.
func GetBuffer() *bytes.Buffer {
	return bytesBufferPool.Get().(*bytes.Buffer)
}

// PutBuffer resets buf and returns it to the pool.
func PutBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bytesBufferPool.Put(buf)
}

// byteSlicePool reuses []byte slices for I/O buffers (e.g. the 8KB head
// read in putBlobForAdd, or zip/tar export buffers).
var byteSlicePool = sync.Pool{
	New: func() any {
		b := make([]byte, 32*1024)
		return &b
	},
}

// GetByteSlice returns a *[]byte from the pool (32KB default capacity).
func GetByteSlice() *[]byte {
	return byteSlicePool.Get().(*[]byte)
}

// PutByteSlice returns the slice to the pool.
func PutByteSlice(buf *[]byte) {
	// Only keep slices up to 1MB to prevent memory leaks from rare large allocations.
	if cap(*buf) > 1024*1024 {
		return
	}
	byteSlicePool.Put(buf)
}
