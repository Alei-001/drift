package core

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
)

var (
	treeMagic = [4]byte{'D', 'R', 'E', 'E'}

	ErrInvalidTree = errors.New("invalid tree file")
	ErrTreeCorrupt = errors.New("tree file corrupted")
)

func (t *Tree) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	if err := t.writeHeader(&buf); err != nil {
		return nil, err
	}

	for i := range t.Entries {
		if err := t.writeEntry(&buf, &t.Entries[i]); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (t *Tree) Unmarshal(data []byte) error {
	r := bytes.NewReader(data)

	if err := t.readHeader(r); err != nil {
		return err
	}

	for i := 0; i < len(t.Entries); i++ {
		if err := t.readEntry(r, &t.Entries[i]); err != nil {
			return err
		}
	}

	return nil
}

func (t *Tree) writeHeader(w io.Writer) error {
	if _, err := w.Write(treeMagic[:]); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, uint32(len(t.Entries)))
}

func (t *Tree) readHeader(r io.Reader) error {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return ErrInvalidTree
	}
	if magic != treeMagic {
		return ErrInvalidTree
	}

	var count uint32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return ErrInvalidTree
	}

	t.Entries = make([]TreeEntry, count)
	return nil
}

func (t *Tree) writeEntry(w io.Writer, entry *TreeEntry) error {
	nameBytes := []byte(entry.Name)
	if len(nameBytes) > 65535 {
		return errors.New("name too long")
	}

	if err := binary.Write(w, binary.LittleEndian, uint16(len(nameBytes))); err != nil {
		return err
	}
	if _, err := w.Write(nameBytes); err != nil {
		return err
	}

	if err := binary.Write(w, binary.LittleEndian, uint8(entry.Type)); err != nil {
		return err
	}

	hashBytes, err := hex.DecodeString(entry.Hash)
	if err != nil {
		return err
	}
	if len(hashBytes) != 32 {
		return errors.New("invalid hash length")
	}
	if _, err := w.Write(hashBytes); err != nil {
		return err
	}

	return binary.Write(w, binary.LittleEndian, entry.Mode)
}

func (t *Tree) readEntry(r io.Reader, entry *TreeEntry) error {
	var nameLen uint16
	if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
		return ErrTreeCorrupt
	}

	nameBytes := make([]byte, nameLen)
	if _, err := io.ReadFull(r, nameBytes); err != nil {
		return ErrTreeCorrupt
	}
	entry.Name = string(nameBytes)

	var objType uint8
	if err := binary.Read(r, binary.LittleEndian, &objType); err != nil {
		return ErrTreeCorrupt
	}
	entry.Type = ObjectType(objType)

	hashBytes := make([]byte, 32)
	if _, err := io.ReadFull(r, hashBytes); err != nil {
		return ErrTreeCorrupt
	}
	entry.Hash = hex.EncodeToString(hashBytes)

	return binary.Read(r, binary.LittleEndian, &entry.Mode)
}
