package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

var (
	indexMagic   = [4]byte{'D', 'R', 'I', 'X'}
	indexVersion = uint32(1)

	ErrInvalidIndex   = errors.New("invalid index file")
	ErrIndexVersion   = errors.New("unsupported index version")
	ErrIndexCorrupt   = errors.New("index file corrupted")
)

func (idx *Index) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	if err := idx.writeHeader(&buf); err != nil {
		return nil, err
	}

	for _, entry := range idx.Entries {
		if err := idx.writeEntry(&buf, &entry); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (idx *Index) Unmarshal(data []byte) error {
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
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(data)*2)
	for i, b := range data {
		result[i*2] = hexChars[b>>4]
		result[i*2+1] = hexChars[b&0x0f]
	}
	return string(result)
}

func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("invalid hex string length: %d", len(s))
	}

	result := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		high, err := hexCharToByte(s[i])
		if err != nil {
			return nil, err
		}
		low, err := hexCharToByte(s[i+1])
		if err != nil {
			return nil, err
		}
		result[i/2] = (high << 4) | low
	}
	return result, nil
}

func hexCharToByte(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	default:
		return 0, fmt.Errorf("invalid hex character: %c", c)
	}
}
