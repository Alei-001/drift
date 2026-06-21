package core

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"time"
)

var (
	commitMagic   = [4]byte{'D', 'C', 'M', 'T'}
	commitVersion = uint32(1)

	ErrInvalidCommit = errors.New("invalid commit file")
	ErrCommitCorrupt = errors.New("commit file corrupted")
)

func (c *Commit) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	if err := c.writeHeader(&buf); err != nil {
		return nil, err
	}

	if err := c.writeString(&buf, c.ID); err != nil {
		return nil, err
	}

	hashBytes, err := hex.DecodeString(c.Hash)
	if err != nil {
		return nil, err
	}
	if _, err := buf.Write(hashBytes); err != nil {
		return nil, err
	}

	treeHashBytes, err := hex.DecodeString(c.TreeHash)
	if err != nil {
		return nil, err
	}
	if _, err := buf.Write(treeHashBytes); err != nil {
		return nil, err
	}

	parentBytes, err := hex.DecodeString(c.Parent)
	if err != nil {
		parentBytes = make([]byte, 32)
	}
	if len(parentBytes) < 32 {
		padded := make([]byte, 32)
		copy(padded[32-len(parentBytes):], parentBytes)
		parentBytes = padded
	}
	if _, err := buf.Write(parentBytes); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.LittleEndian, c.Timestamp.UnixMilli()); err != nil {
		return nil, err
	}

	if err := c.writeString(&buf, c.Branch); err != nil {
		return nil, err
	}

	if err := c.writeString(&buf, c.Message); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *Commit) Unmarshal(data []byte) error {
	r := bytes.NewReader(data)

	if err := c.readHeader(r); err != nil {
		return err
	}

	id, err := c.readString(r)
	if err != nil {
		return err
	}
	c.ID = id

	hashBytes := make([]byte, 32)
	if _, err := io.ReadFull(r, hashBytes); err != nil {
		return ErrCommitCorrupt
	}
	c.Hash = hex.EncodeToString(hashBytes)

	treeHashBytes := make([]byte, 32)
	if _, err := io.ReadFull(r, treeHashBytes); err != nil {
		return ErrCommitCorrupt
	}
	c.TreeHash = hex.EncodeToString(treeHashBytes)

	parentBytes := make([]byte, 32)
	if _, err := io.ReadFull(r, parentBytes); err != nil {
		return ErrCommitCorrupt
	}
	parent := hex.EncodeToString(parentBytes)
	if parent != "0000000000000000000000000000000000000000000000000000000000000000" {
		c.Parent = parent
	}

	var timestamp int64
	if err := binary.Read(r, binary.LittleEndian, &timestamp); err != nil {
		return ErrCommitCorrupt
	}
	c.Timestamp = time.UnixMilli(timestamp)

	branch, err := c.readString(r)
	if err != nil {
		return err
	}
	c.Branch = branch

	message, err := c.readString(r)
	if err != nil {
		return err
	}
	c.Message = message

	return nil
}

func (c *Commit) writeHeader(w io.Writer) error {
	if _, err := w.Write(commitMagic[:]); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, commitVersion)
}

func (c *Commit) readHeader(r io.Reader) error {
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return ErrInvalidCommit
	}
	if magic != commitMagic {
		return ErrInvalidCommit
	}

	var version uint32
	if err := binary.Read(r, binary.LittleEndian, &version); err != nil {
		return ErrInvalidCommit
	}
	if version != commitVersion {
		return ErrInvalidCommit
	}

	return nil
}

func (c *Commit) writeString(w io.Writer, s string) error {
	b := []byte(s)
	if len(b) > 65535 {
		return errors.New("string too long")
	}
	if err := binary.Write(w, binary.LittleEndian, uint16(len(b))); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func (c *Commit) readString(r io.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", ErrCommitCorrupt
	}
	b := make([]byte, length)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", ErrCommitCorrupt
	}
	return string(b), nil
}
