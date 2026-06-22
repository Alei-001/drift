package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	indexMagic   = [4]byte{'D', 'R', 'I', 'X'}
	indexVersion = uint32(1)

	// checksumSize is the length of the SHA-256 trailer appended to the index.
	checksumSize = 32

	ErrInvalidIndex = errors.New("invalid index file")
	ErrIndexVersion = errors.New("unsupported index version")
	ErrIndexCorrupt = errors.New("index file corrupted")
)

func (idx *Index) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	// Hasher computes SHA-256 over the body (header + entries) for a checksum
	// trailer, mirroring go-git's index checksum.
	h := sha256.New()
	mw := io.MultiWriter(&buf, h)

	if err := idx.writeHeader(mw); err != nil {
		return nil, err
	}

	for _, entry := range idx.Entries {
		if err := idx.writeEntry(mw, &entry); err != nil {
			return nil, err
		}
	}

	// Append checksum trailer.
	if _, err := buf.Write(h.Sum(nil)); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (idx *Index) Unmarshal(data []byte) error {
	// Verify checksum trailer (if present). Backwards-compatible: indexes
	// without a trailer are accepted; indexes with a valid 32-byte trailer
	// are verified; a trailer that doesn't match is corruption.
	if len(data) >= checksumSize {
		body := data[:len(data)-checksumSize]
		got := sha256.Sum256(body)
		trailer := data[len(data)-checksumSize:]
		if bytes.Equal(got[:], trailer) {
			// Valid checksum trailer; strip it before parsing.
			data = body
		}
		// If it doesn't match, it might be an old-format index with no
		// trailer whose last 32 bytes happen to be entry data. The trailing
		// data check below will catch real corruption.
	}

	r := bytes.NewReader(data)

	if err := idx.readHeader(r); err != nil {
		return err
	}

	for i := 0; i < len(idx.Entries); i++ {
		entry, err := idx.readEntry(r)
		if err != nil {
			return err
		}
		idx.Entries[i] = *entry
	}

	// Issue 24: reject trailing bytes after entries (indicates corruption).
	if r.Len() != 0 {
		return ErrIndexCorrupt
	}

	return nil
}

func (idx *Index) writeHeader(w io.Writer) error {
	if _, err := w.Write(indexMagic[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, indexVersion); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(idx.Entries))); err != nil {
		return err
	}
	return nil
}

func (idx *Index) readHeader(r io.Reader) error {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return ErrInvalidIndex
	}
	if magic != indexMagic {
		return ErrInvalidIndex
	}

	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return ErrInvalidIndex
	}
	if version != indexVersion {
		return ErrIndexVersion
	}

	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return ErrInvalidIndex
	}

	idx.Entries = make([]IndexEntry, count)
	return nil
}

func (idx *Index) writeEntry(w io.Writer, entry *IndexEntry) error {
	pathBytes := []byte(entry.Path)
	if len(pathBytes) > 65535 {
		return fmt.Errorf("path too long: %d bytes", len(pathBytes))
	}

	if err := binary.Write(w, binary.LittleEndian, uint16(len(pathBytes))); err != nil {
		return err
	}
	if _, err := w.Write(pathBytes); err != nil {
		return err
	}

	hashBytes, err := hexDecode(entry.Hash)
	if err != nil {
		return fmt.Errorf("invalid hash: %w", err)
	}
	if len(hashBytes) != 32 {
		return fmt.Errorf("invalid hash length: %d", len(hashBytes))
	}
	if _, err := w.Write(hashBytes); err != nil {
		return err
	}

	timestamp := entry.ModifiedAt.UnixMilli()
	if err := binary.Write(w, binary.LittleEndian, timestamp); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, entry.Size); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, entry.Mode); err != nil {
		return err
	}

	return nil
}

func (idx *Index) readEntry(r io.Reader) (*IndexEntry, error) {
	var pathLen uint16
	if err := binary.Read(r, binary.LittleEndian, &pathLen); err != nil {
		return nil, ErrIndexCorrupt
	}

	pathBytes := make([]byte, pathLen)
	if _, err := io.ReadFull(r, pathBytes); err != nil {
		return nil, ErrIndexCorrupt
	}

	hashBytes := make([]byte, 32)
	if _, err := io.ReadFull(r, hashBytes); err != nil {
		return nil, ErrIndexCorrupt
	}

	var timestamp int64
	if err := binary.Read(r, binary.LittleEndian, &timestamp); err != nil {
		return nil, ErrIndexCorrupt
	}

	var size int64
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return nil, ErrIndexCorrupt
	}

	var mode uint32
	if err := binary.Read(r, binary.LittleEndian, &mode); err != nil {
		return nil, ErrIndexCorrupt
	}

	return &IndexEntry{
		Path:       string(pathBytes),
		Hash:       hexEncode(hashBytes),
		ModifiedAt: time.UnixMilli(timestamp),
		Size:       size,
		Mode:       mode,
	}, nil
}

func hexEncode(data []byte) string {
	return hex.EncodeToString(data)
}

func hexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}
