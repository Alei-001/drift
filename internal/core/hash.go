package core

import (
	"bytes"
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

	head := make([]byte, 8192)
	n, readErr := io.ReadFull(f, head)
	if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
		return "", readErr
	}
	head = head[:n]

	if bytes.Contains(head, []byte{0}) {
		r := io.MultiReader(bytes.NewReader(head), f)
		h := sha256.New()
		if _, err := io.Copy(h, r); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}

	rest, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	data := append(head, rest...)
	return CalculateHash(bytes.ReplaceAll(data, []byte{'\r', '\n'}, []byte{'\n'})), nil
}

// CalculateHashFromFileLF reads the file at path, normalizes CRLF→LF line
// endings, and returns the SHA-256 hash. Used for comparing working-tree
// files against LF-normalized blobs on Windows with autocrlf enabled.
func CalculateHashFromFileLF(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return CalculateHash(bytes.ReplaceAll(data, []byte{'\r', '\n'}, []byte{'\n'})), nil
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
