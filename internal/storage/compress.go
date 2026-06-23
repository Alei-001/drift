package storage

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

// Compressed object format:
//   magic: 4 bytes "DRZL"
//   version: 1 byte (currently 1)
//   original size: uint32 LE (for pre-allocation)
//   zlib-compressed payload
//
// Objects without the DRZL magic are treated as raw (uncompressed) for
// backward compatibility with repositories created before compression
// was introduced.

const (
	compressedMagic    = "DRZL"
	compressedVersion  = 1
	compressedHeaderSz = 4 + 1 + 4 // magic + version + size
)

// ErrCorruptedObject is returned when an object file cannot be parsed
// as either compressed or raw format.
var ErrCorruptedObject = errors.New("corrupted object file")

// compressBytes compresses data using zlib with the DRZL header.
func compressBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	// Pre-allocate to avoid reallocation.
	buf.Grow(compressedHeaderSz + len(data)/2)

	// Write header.
	buf.WriteString(compressedMagic)
	buf.WriteByte(compressedVersion)
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(data))); err != nil {
		return nil, err
	}

	// Compress payload.
	zw, err := zlib.NewWriterLevel(&buf, zlib.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(data); err != nil {
		zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompressBytes checks for the DRZL magic and decompresses if present.
// If the data does not start with the magic, it is returned as-is (raw
// format, backward compatible).
func decompressBytes(data []byte) ([]byte, error) {
	if len(data) < compressedHeaderSz {
		// Too short to have a header — treat as raw.
		return data, nil
	}

	if string(data[:4]) != compressedMagic {
		// Not compressed — raw format.
		return data, nil
	}

	// Parse header.
	version := data[4]
	if version != compressedVersion {
		return nil, ErrCorruptedObject
	}

	origSize := binary.LittleEndian.Uint32(data[5:9])
	payload := data[9:]

	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	out := make([]byte, 0, origSize)
	buf := make([]byte, 32*1024)
	for {
		n, err := zr.Read(buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// compressFileToPath writes a compressed version of srcData to the given
// path atomically (via tmp + rename).
func compressFileToPath(path string, srcData []byte) error {
	compressed, err := compressBytes(srcData)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, compressed, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// readAndDecompress reads a file and decompresses it if needed.
func readAndDecompress(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decompressBytes(raw)
}

// streamingDecompressReader wraps a reader that may contain compressed
// data. It peeks at the first 4 bytes to detect the DRZL magic.
// For compressed data, it returns a reader that decompresses on the fly.
// For raw data, it returns a reader that replays the peeked bytes + the
// rest of the original reader.
func streamingDecompressReader(r io.Reader) (io.Reader, error) {
	// Peek the header.
	header := make([]byte, compressedHeaderSz)
	n, err := io.ReadFull(r, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	header = header[:n]

	// If we couldn't read a full header, or the magic doesn't match,
	// treat as raw: replay the bytes we read + the rest.
	if n < compressedHeaderSz || string(header[:4]) != compressedMagic {
		return io.MultiReader(bytes.NewReader(header), r), nil
	}

	// Parse compressed header.
	version := header[4]
	if version != compressedVersion {
		return nil, ErrCorruptedObject
	}

	zr, err := zlib.NewReader(io.MultiReader(bytes.NewReader(header[9:]), r))
	if err != nil {
		return nil, err
	}

	return zr, nil
}
