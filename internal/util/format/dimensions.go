package format

import (
	"bytes"
	"encoding/binary"
	"image"
	_ "image/gif"  // register GIF decoder for DecodeDimensions
	_ "image/jpeg" // register JPEG decoder for DecodeDimensions
	_ "image/png"  // register PNG decoder for DecodeDimensions
)

// imageFormat identifies an image type by magic byte detection.
type imageFormat string

const (
	imgPNG  imageFormat = "png"
	imgJPEG imageFormat = "jpg"
	imgGIF  imageFormat = "gif"
	imgWebP imageFormat = "webp"
	imgBMP  imageFormat = "bmp"
	imgTIFF imageFormat = "tiff"
)

// minBMPHeaderSize is the minimum header length for a valid BMP file:
// 14-byte file header + 40-byte BITMAPINFOHEADER (the most common DIB
// header). Shorter headers cannot represent a standard BMP.
const minBMPHeaderSize = 54

// pngSignature is the 8-byte PNG magic: \x89PNG\r\n\x1a\n.
var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// detectImageFormat returns the canonical format key if the header matches
// a known image magic byte sequence, or "" if unknown.
func detectImageFormat(header []byte) imageFormat {
	if len(header) == 0 {
		return ""
	}
	// PNG: 8-byte signature \x89PNG\r\n\x1a\n
	if len(header) >= 8 && bytes.Equal(header[:8], pngSignature) {
		return imgPNG
	}
	// JPEG: \xFF\xD8\xFF
	if len(header) >= 3 && header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return imgJPEG
	}
	// GIF: GIF8 (covers GIF87a and GIF89a)
	if len(header) >= 4 && string(header[:4]) == "GIF8" {
		return imgGIF
	}
	// WebP: RIFF....WEBP
	if len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
		return imgWebP
	}
	// BMP: "BM" magic plus a valid DIB header size at offset 14-17.
	// Requiring minBMPHeaderSize and a valid DIB size avoids false positives
	// on text files that happen to start with "BM" (e.g. "BMW parts...").
	// Known valid DIB header sizes: 12 (BITMAPCOREHEADER), 40
	// (BITMAPINFOHEADER), 52, 56, 64, 108 (BITMAPV4HEADER), 124
	// (BITMAPV5HEADER).
	if len(header) >= minBMPHeaderSize && header[0] == 'B' && header[1] == 'M' {
		dibSize := binary.LittleEndian.Uint32(header[14:18])
		switch dibSize {
		case 12, 40, 52, 56, 64, 108, 124:
			return imgBMP
		}
	}
	// TIFF: II*\x00 (little-endian) or MM\x00* (big-endian)
	if len(header) >= 4 {
		if header[0] == 'I' && header[1] == 'I' && header[2] == 0x2A && header[3] == 0x00 {
			return imgTIFF
		}
		if header[0] == 'M' && header[1] == 'M' && header[2] == 0x00 && header[3] == 0x2A {
			return imgTIFF
		}
	}
	return ""
}

// DecodeDimensions parses image dimensions from the header bytes. It
// supports PNG, JPEG, GIF (via image.DecodeConfig), and WebP, BMP, TIFF
// (via manual magic-byte parsing). Returns 0x0 if the format is not
// recognized or the header is too short to extract dimensions.
func DecodeDimensions(data []byte) (int, int) {
	switch detectImageFormat(data) {
	case imgWebP:
		return webpDimensions(data)
	case imgBMP:
		return bmpDimensions(data)
	case imgTIFF:
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
// BITMAPINFOHEADER and later (32-bit dimensions at offsets 18/22). A
// negative height in BITMAPINFOHEADER indicates a top-down bitmap; the
// absolute value is returned.
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
	w := int(int32(binary.LittleEndian.Uint32(data[18:22])))
	hRaw := int32(binary.LittleEndian.Uint32(data[22:26]))
	if hRaw < 0 {
		hRaw = -hRaw // top-down bitmap
	}
	return w, int(hRaw)
}

// tiffDimensions parses the ImageWidth (0x0100) and ImageLength (0x0101) tags
// from the first IFD of a TIFF file. Both little-endian (II) and big-endian
// (MM) byte orders are supported. All offsets are handled as int64 to avoid
// panics on 32-bit platforms where a large uint32 IFD offset would overflow
// int and become negative, causing an out-of-bounds index.
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
	ifdOff := int64(order.Uint32(data[4:8]))
	if ifdOff < 0 || ifdOff > int64(len(data)) {
		return 0, 0
	}
	if ifdOff+2 > int64(len(data)) {
		return 0, 0
	}
	ifdOffInt := int(ifdOff) // safe: ifdOff <= len(data) which fits in int
	numEntries := int(order.Uint16(data[ifdOffInt : ifdOffInt+2]))
	if ifdOffInt+2+numEntries*12 > len(data) {
		return 0, 0
	}
	var width, height int
	for i := 0; i < numEntries; i++ {
		entry := ifdOffInt + 2 + i*12
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
// ImageWidth/ImageLength encodings. Returns 0 for out-of-range offsets or
// unsupported types.
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
		return int(int32(order.Uint32(data[off : off+4])))
	}
	return 0
}
