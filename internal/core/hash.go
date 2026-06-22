package core

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
)

func CalculateHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func CalculateHashFromFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// NewHasher returns a new SHA-256 hasher implementing hash.Hash.
// Used for streaming hash verification of large blobs.
func NewHasher() hash.Hash {
	return sha256.New()
}

// HexSum returns the hex-encoded sum of the hasher. It does not affect the
// hasher's state; call Sum(nil) on the underlying hash.Hash to get raw bytes.
func HexSum(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}
