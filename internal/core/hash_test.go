package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// TestCalculateHash_Empty verifies that hashing empty input yields the SHA-256 of empty data.
func TestCalculateHash_Empty(t *testing.T) {
	got := CalculateHash(nil)
	want := sha256.Sum256(nil)
	if got != hex.EncodeToString(want[:]) {
		t.Fatalf("CalculateHash(nil) = %q, want %q", got, hex.EncodeToString(want[:]))
	}
}

// TestCalculateHash_Deterministic verifies that identical inputs produce identical hashes.
func TestCalculateHash_Deterministic(t *testing.T) {
	a := CalculateHash([]byte("drift"))
	b := CalculateHash([]byte("drift"))
	if a != b {
		t.Fatalf("identical inputs produced different hashes: %q vs %q", a, b)
	}
}

// TestCalculateHash_DifferentInputs verifies that different inputs produce different hashes.
func TestCalculateHash_DifferentInputs(t *testing.T) {
	a := CalculateHash([]byte("drift"))
	b := CalculateHash([]byte("Drift"))
	if a == b {
		t.Fatalf("different inputs produced the same hash: %q", a)
	}
}

// TestCalculateHash_Length verifies the hash is 64 hex characters (256 bits).
func TestCalculateHash_Length(t *testing.T) {
	got := CalculateHash([]byte("x"))
	if len(got) != 64 {
		t.Fatalf("hash length = %d, want 64", len(got))
	}
}

// TestCalculateHashFromFile_ExistingFile verifies hashing a file matches in-memory hashing.
func TestCalculateHashFromFile_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	data := []byte("hello drift")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := CalculateHashFromFile(path)
	if err != nil {
		t.Fatalf("CalculateHashFromFile returned error: %v", err)
	}
	want := CalculateHash(data)
	if got != want {
		t.Fatalf("CalculateHashFromFile = %q, want %q", got, want)
	}
}

// TestCalculateHashFromFile_MissingFile verifies that opening a missing file returns an error.
func TestCalculateHashFromFile_MissingFile(t *testing.T) {
	_, err := CalculateHashFromFile(filepath.Join(t.TempDir(), "nope.bin"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestCalculateHashFromFile_LargeFile verifies that stream hashing works on multi-chunk files.
func TestCalculateHashFromFile_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	data := make([]byte, 1024*1024) // 1 MB, larger than a single read buffer
	for i := range data {
		data[i] = byte(i % 251)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := CalculateHashFromFile(path)
	if err != nil {
		t.Fatalf("CalculateHashFromFile returned error: %v", err)
	}
	want := CalculateHash(data)
	if got != want {
		t.Fatalf("large file hash mismatch: got %q, want %q", got, want)
	}
}
