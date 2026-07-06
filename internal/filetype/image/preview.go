package image

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	sizefmt "github.com/your-org/drift/internal/util/format"
)

// previewFormatName maps canonical format keys to display names.
var previewFormatName = map[string]string{
	formatPNG:  "PNG",
	formatJPEG: "JPEG",
	formatGIF:  "GIF",
	formatWebP: "WebP",
	formatBMP:  "BMP",
	formatTIFF: "TIFF",
}

// Preview returns a one-line summary: format name, dimensions, and
// human-readable file size (e.g. "PNG 1920x1080 2.4 MB").
//
// Only the header is inspected (for magic detection and dimension parsing)
// and size is taken from the caller; the content reader is never touched.
// This keeps memory constant for arbitrarily large images.
func (e *ImageEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
	_ = reader
	_ = maxLines
	format := detectFormatByMagic(header)
	name := previewFormatName[format]
	if name == "" {
		name = "image"
	}
	w, h := decodeDimensions(header)
	return fmt.Sprintf("%s %dx%d %s", name, w, h, sizefmt.Bytes(int64(size))), nil
}

// decodeDimensions parses image dimensions from the header. WebP, BMP, and
// TIFF are decoded manually because the Go standard library does not register
// decoders for them by default. PNG, JPEG, and GIF use image.DecodeConfig.
// Returns 0x0 if the format is not decodable.
func decodeDimensions(data []byte) (int, int) {
	switch detectFormatByMagic(data) {
	case formatWebP:
		return webpDimensions(data)
	case formatBMP:
		return bmpDimensions(data)
	case formatTIFF:
		return tiffDimensions(data)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// webpDimensions parses canvas/frame dimensions from a WebP bitstream. It
// supports the three VP8 chunk variants: VP8 (lossy), VP8L (lossless), and
// VP8X (extended). Returns 0x0 if the data is too short or the chunk type is
// unknown.
func webpDimensions(data []byte) (int, int) {
	// RIFF....WEBP at offset 0; the chunk FourCC begins at offset 12.
	if len(data) < 16 {
		return 0, 0
	}
	switch string(data[12:16]) {
	case "VP8 ":
		// Lossy. After the 8-byte chunk header (offset 20): 3-byte frame tag,
		// 3-byte start code, then 16-bit LE width|scale and height|scale. The
		// lower 14 bits of each are width-1 / height-1.
		if len(data) < 30 {
			return 0, 0
		}
		w := int(binary.LittleEndian.Uint16(data[26:28]) & 0x3FFF)
		h := int(binary.LittleEndian.Uint16(data[28:30]) & 0x3FFF)
		return w + 1, h + 1
	case "VP8L":
		// Lossless. After the 8-byte chunk header: 1-byte signature (0x2F),
		// then a bit-packed header: 1 bit version, 1 bit alpha, 14 bits
		// width-1, 14 bits height-1 (LSB-first within each byte).
		if len(data) < 25 {
			return 0, 0
		}
		v := binary.LittleEndian.Uint32(data[21:25])
		w := int((v >> 2) & 0x3FFF)
		h := int((v >> 16) & 0x3FFF)
		return w + 1, h + 1
	case "VP8X":
		// Extended. After the 8-byte chunk header (offset 20): 1-byte flags,
		// 3-byte reserved, then 24-bit LE canvas width-1 and height-1.
		if len(data) < 30 {
			return 0, 0
		}
		w := int(data[24]) | int(data[25])<<8 | int(data[26])<<16
		h := int(data[27]) | int(data[28])<<8 | int(data[29])<<16
		return w + 1, h + 1
	}
	return 0, 0
}

// bmpDimensions parses pixel dimensions from a BMP DIB header. It handles
// BITMAPCOREHEADER (DIB size 12, 16-bit dimensions at offsets 18/20) and
// BITMAPINFOHEADER and later (32-bit dimensions at offsets 18/22).
func bmpDimensions(data []byte) (int, int) {
	if len(data) < 18 {
		return 0, 0
	}
	dibSize := binary.LittleEndian.Uint32(data[14:18])
	if dibSize == 12 {
		// BITMAPCOREHEADER: 16-bit width/height at offsets 18/20
		if len(data) < 22 {
			return 0, 0
		}
		w := int(binary.LittleEndian.Uint16(data[18:20]))
		h := int(binary.LittleEndian.Uint16(data[20:22]))
		return w, h
	}
	// BITMAPINFOHEADER and later: 32-bit width/height at offsets 18/22
	if len(data) < 26 {
		return 0, 0
	}
	w := int(binary.LittleEndian.Uint32(data[18:22]))
	h := int(binary.LittleEndian.Uint32(data[22:26]))
	return w, h
}

// tiffDimensions parses the ImageWidth (0x0100) and ImageLength (0x0101) tags
// from the first IFD of a TIFF file. Both little-endian (II) and big-endian
// (MM) byte orders are supported.
func tiffDimensions(data []byte) (int, int) {
	if len(data) < 8 {
		return 0, 0
	}
	var order binary.ByteOrder
	switch {
	case data[0] == 'I' && data[1] == 'I':
		order = binary.LittleEndian
	case data[0] == 'M' && data[1] == 'M':
		order = binary.BigEndian
	default:
		return 0, 0
	}
	ifdOff := int(order.Uint32(data[4:8]))
	if ifdOff+2 > len(data) {
		return 0, 0
	}
	numEntries := int(order.Uint16(data[ifdOff : ifdOff+2]))
	if ifdOff+2+numEntries*12 > len(data) {
		return 0, 0
	}
	var width, height int
	for i := 0; i < numEntries; i++ {
		entry := ifdOff + 2 + i*12
		tag := order.Uint16(data[entry : entry+2])
		if tag != 0x0100 && tag != 0x0101 {
			continue
		}
		typ := order.Uint16(data[entry+2 : entry+4])
		val := readTIFFTagValue(data, typ, entry+8, order)
		if tag == 0x0100 {
			width = val
		} else {
			height = val
		}
	}
	return width, height
}

// readTIFFTagValue reads an inline IFD entry value for the dimension tags.
// Only SHORT (type 3) and LONG (type 4) are handled, which covers all
// ImageWidth/ImageLength encodings.
func readTIFFTagValue(data []byte, typ uint16, off int, order binary.ByteOrder) int {
	switch typ {
	case 3: // SHORT (uint16)
		if off+2 > len(data) {
			return 0
		}
		return int(order.Uint16(data[off : off+2]))
	case 4: // LONG (uint32)
		if off+4 > len(data) {
			return 0
		}
		return int(order.Uint32(data[off : off+4]))
	}
	return 0
}
