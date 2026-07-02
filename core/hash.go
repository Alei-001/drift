package core

import (
	"encoding/hex"
)

// Hash is a BLAKE3 hash (32 bytes).
type Hash [32]byte

// HashSize is the length of a Hash in bytes.
const HashSize = 32

// String returns the hex representation truncated to the first 8 characters.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])[:8]
}

// FullString returns the full 64-character hex representation.
func (h Hash) FullString() string {
	return hex.EncodeToString(h[:])
}

// IsZero returns true if the hash is all zeros.
func (h Hash) IsZero() bool {
	for _, b := range h {
		if b != 0 {
			return false
		}
	}
	return true
}
