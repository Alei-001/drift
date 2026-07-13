package video

import (
	"encoding/binary"
	"fmt"
	"io"

	sizefmt "github.com/Alei-001/drift/internal/util/format"
)

// maxVideoPreviewSize is the threshold above which dimension parsing is
// skipped. Real-world MP4s often store the moov atom at the end of the
// file, so for large files the header alone is insufficient to extract
// dimensions. Only format detection (from the leading magic bytes) and
// the caller-supplied size are used; the content reader is never consumed.
const maxVideoPreviewSize = 100 * 1024 * 1024 // 100 MB

// Preview returns a short, human-readable summary of a video file.
// Format: "<FORMAT> <WxH> <SIZE>" when dimensions are available, otherwise
// "<FORMAT> <SIZE>". Only MP4/MOV containers expose dimensions; for other
// formats the size-only form is returned.
//
// Only the header is inspected (format detection and, for MP4, a best-effort
// scan of the leading boxes for a tkhd) and size is taken from the caller.
// The content reader is never read, so previewing a 500 MB video stays in
// constant memory. Files larger than maxVideoPreviewSize skip dimension
// parsing entirely since the moov atom is unlikely to be in the header.
func (e *VideoEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
	_ = reader
	_ = maxLines
	format := detectVideoFormat(header)
	if format == "" {
		format = "VIDEO"
	}
	sizeStr := sizefmt.Bytes(size)

	// For oversized files, skip dimension parsing — the header is too small
	// to contain the moov atom, and the reader is never consumed to keep
	// memory constant.
	if size > maxVideoPreviewSize {
		return fmt.Sprintf("%s %s", format, sizeStr), nil
	}

	if format == "MP4" {
		if w, h, ok := parseMP4Dimensions(header); ok && w > 0 && h > 0 {
			return fmt.Sprintf("%s %dx%d %s", format, w, h, sizeStr), nil
		}
	}
	return fmt.Sprintf("%s %s", format, sizeStr), nil
}

// parseMP4Dimensions walks the MP4 top-level box hierarchy looking for the
// first tkhd box (moov -> trak -> tkhd) with non-zero dimensions and reads
// its width/height fields. This skips audio tracks (which report 0x0) and
// continues searching until a video track is found. Returns ok=false on
// any malformation; never panics on truncated data.
func parseMP4Dimensions(data []byte) (width, height int, ok bool) {
	var dims struct{ w, h int }
	found := walkBoxes(data, func(boxType string, payload []byte) bool {
		if boxType != "moov" {
			return false
		}
		return walkBoxes(payload, func(t string, p []byte) bool {
			if t != "trak" {
				return false
			}
			return walkBoxes(p, func(t2 string, p2 []byte) bool {
				if t2 != "tkhd" {
					return false
				}
				if w, h, k := parseTkhd(p2); k && w > 0 && h > 0 {
					dims.w, dims.h, ok = w, h, true
					return true
				}
				return false
			})
		})
	})
	if !found {
		return 0, 0, false
	}
	return dims.w, dims.h, true
}

// walkBoxes iterates over sequential ISO-BMFF boxes in data, invoking fn for
// each box with its type and payload. If fn returns true, iteration stops and
// walkBoxes returns true. Malformed sizes or truncated data stop iteration
// safely.
func walkBoxes(data []byte, fn func(boxType string, payload []byte) bool) bool {
	offset := 0
	for offset+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		boxType := string(data[offset+4 : offset+8])

		var boxEnd, headerSize int
		switch {
		case size == 0:
			// Box extends to end of data.
			boxEnd = len(data)
			headerSize = 8
		case size == 1:
			// 64-bit large size follows the type.
			if offset+16 > len(data) {
				return false
			}
			size = int(binary.BigEndian.Uint64(data[offset+8 : offset+16]))
			boxEnd = offset + size
			headerSize = 16
		default:
			boxEnd = offset + size
			headerSize = 8
		}

		// size==0 is a special ISO-BMFF sentinel meaning "box extends to
		// end of file" (common in streaming MP4 mdat). It is not an actual
		// size, so skip the headerSize sanity check for it.
		if size != 0 && size < headerSize {
			return false
		}
		if boxEnd > len(data) || boxEnd < offset {
			return false
		}
		if fn(boxType, data[offset+headerSize:boxEnd]) {
			return true
		}
		offset = boxEnd
	}
	return false
}

// parseTkhd reads width/height from a tkhd box payload (the bytes after the
// 8-byte box header). Width and height are stored as 32-bit fixed-point 16.16
// values; the integer part (upper 16 bits) is returned.
func parseTkhd(payload []byte) (width, height int, ok bool) {
	if len(payload) < 4 {
		return 0, 0, false
	}
	version := payload[0]
	// After version+flags (4 bytes):
	//  v0: creation(4) modification(4) trackID(4) reserved(4) duration(4)
	//      reserved(8) layer(2) altGroup(2) volume(2) reserved(2) matrix(36)
	//      = 72 bytes before width.
	//  v1: creation(8) modification(8) trackID(4) reserved(4) duration(8)
	//      reserved(8) layer(2) altGroup(2) volume(2) reserved(2) matrix(36)
	//      = 88 bytes before width.
	var widthOffset int
	if version == 1 {
		widthOffset = 4 + 88
	} else {
		widthOffset = 4 + 72
	}
	if len(payload) < widthOffset+8 {
		return 0, 0, false
	}
	w := binary.BigEndian.Uint32(payload[widthOffset : widthOffset+4])
	h := binary.BigEndian.Uint32(payload[widthOffset+4 : widthOffset+8])
	return int(w >> 16), int(h >> 16), true
}
