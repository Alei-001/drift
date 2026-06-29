package video

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// buildBox assembles an ISO-BMFF box: 4-byte size + 4-byte type + payload.
func buildBox(boxType string, payload []byte) []byte {
	buf := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(8+len(payload)))
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

// buildMP4WithDimensions constructs a minimal MP4 byte stream whose first
// trak/tkhd reports the given pixel dimensions. The layout is:
//   ftyp | moov(trak(tkhd))
func buildMP4WithDimensions(width, height int) []byte {
	// tkhd payload, version 0 (84 bytes). Width/height are fixed 16.16 at
	// payload offsets 76 and 80.
	tkhdPayload := make([]byte, 84)
	tkhdPayload[0] = 0 // version 0
	binary.BigEndian.PutUint32(tkhdPayload[76:80], uint32(width)<<16)
	binary.BigEndian.PutUint32(tkhdPayload[80:84], uint32(height)<<16)

	tkhdBox := buildBox("tkhd", tkhdPayload)
	trakBox := buildBox("trak", tkhdBox)
	moovBox := buildBox("moov", trakBox)
	// ftyp: major_brand(4) + minor_version(4) + compatible_brand(4).
	ftypPayload := []byte{'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm'}
	ftypBox := buildBox("ftyp", ftypPayload)

	var buf bytes.Buffer
	buf.Write(ftypBox)
	buf.Write(moovBox)
	return buf.Bytes()
}

func TestDetectByExtension(t *testing.T) {
	engine := NewEngine()
	tests := []string{
		"clip.mp4",
		"clip.MP4", // case-insensitive
		"clip.mov",
		"clip.avi",
		"clip.mkv",
		"clip.webm",
	}
	for _, path := range tests {
		if !engine.DetectByExtension(path) {
			t.Errorf("DetectByExtension(%q) = false, want true", path)
		}
	}
}

func TestDetectByMagic(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		name string
		data []byte
	}{
		{
			name: "mp4",
			// size=0x18, type="ftyp", brand="mp42", minor=0, compat="isom"
			data: []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00isom"),
		},
		{
			name: "mov",
			// MOV also uses an ftyp box; brand "qt  ".
			data: []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00"),
		},
		{
			name: "avi",
			data: []byte("RIFF\x00\x00\x00\x00AVI LIST\x00\x00\x00\x00"),
		},
		{
			name: "mkv",
			data: append([]byte{0x1A, 0x45, 0xDF, 0xA3}, make([]byte, 16)...),
		},
		{
			name: "webm",
			// EBML header containing a DocType "webm" string.
			data: append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00}, []byte("webm")...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !engine.DetectByMagic(tt.data) {
				t.Errorf("DetectByMagic(%s magic) = false, want true", tt.name)
			}
		})
	}
}

func TestDetectUnknownFormat(t *testing.T) {
	engine := NewEngine()
	// Plain text file with no video magic bytes: neither extension nor magic
	// should match.
	if engine.DetectByExtension("notes.txt") {
		t.Error("DetectByExtension(notes.txt) = true, want false")
	}
	if engine.DetectByMagic([]byte("hello world\n")) {
		t.Error("DetectByMagic(hello world) = true, want false")
	}
	// Unknown extension with no recognizable video header.
	if engine.DetectByExtension("archive.zip") {
		t.Error("DetectByExtension(archive.zip) = true, want false")
	}
	if engine.DetectByMagic([]byte{0x50, 0x4B, 0x03, 0x04}) {
		t.Error("DetectByMagic(zip header) = true, want false")
	}
}

func TestDetectByHeuristic(t *testing.T) {
	engine := NewEngine()
	// Videos are never sniffed heuristically; this layer always returns false.
	tests := []struct {
		name   string
		path   string
		header []byte
	}{
		{"video magic bytes", "unknown.bin", []byte("\x00\x00\x00\x18ftypmp42")},
		{"plain text", "noext", []byte("hello world")},
		{"empty", "empty.dat", []byte{}},
		{"video extension", "clip.mp4", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.DetectByHeuristic(tc.path, tc.header); got != false {
				t.Errorf("DetectByHeuristic(%q, ...) = %v, want false", tc.name, got)
			}
		})
	}
}

func TestDiffSizeChanged(t *testing.T) {
	engine := NewEngine()
	oldData := bytes.Repeat([]byte{0x01}, 100)
	newData := bytes.Repeat([]byte{0x02}, 200)

	diff, err := engine.Diff("old.mp4", oldData, "new.mp4", newData)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	expected := "video file size changed: 100 B -> 200 B"
	if diff != expected {
		t.Errorf("Diff = %q, want %q", diff, expected)
	}
}

func TestDiffNoChange(t *testing.T) {
	engine := NewEngine()
	data := bytes.Repeat([]byte{0x01}, 100)

	diff, err := engine.Diff("old.mp4", data, "new.mp4", data)
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if diff != "" {
		t.Errorf("Diff = %q, want empty string for identical data", diff)
	}
}

func TestPreviewWithDimensions(t *testing.T) {
	engine := NewEngine()
	data := buildMP4WithDimensions(1920, 1080)
	preview := engine.Preview(data, 10)

	// Expected form: "MP4 1920x1080 <size>".
	if !strings.HasPrefix(preview, "MP4 1920x1080 ") {
		t.Errorf("Preview = %q, want prefix %q", preview, "MP4 1920x1080 ")
	}
	// Should include the human-readable size (in bytes for this small sample).
	if !strings.HasSuffix(preview, " B") {
		t.Errorf("Preview = %q, want suffix indicating byte size", preview)
	}
}

func TestPreviewWithoutDimensions(t *testing.T) {
	engine := NewEngine()
	// AVI header: detectable as AVI but carries no parseable dimensions.
	aviData := []byte("RIFF\x00\x00\x00\x00AVI LIST\x00\x00\x00\x00")
	preview := engine.Preview(aviData, 10)

	// Should start with format name and contain size, but no "WxH".
	if !strings.HasPrefix(preview, "AVI ") {
		t.Errorf("Preview = %q, want prefix %q", preview, "AVI ")
	}
	if strings.Contains(preview, "x") {
		t.Errorf("Preview = %q, should not contain dimensions", preview)
	}
}

func TestPreviewMKVFormat(t *testing.T) {
	engine := NewEngine()
	// EBML header with a matroska DocType.
	mkvData := append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00}, []byte("matroska")...)
	preview := engine.Preview(mkvData, 10)
	if !strings.HasPrefix(preview, "MKV ") {
		t.Errorf("Preview = %q, want prefix %q", preview, "MKV ")
	}
}

func TestPreviewWebMFormat(t *testing.T) {
	engine := NewEngine()
	webmData := append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x00, 0x00, 0x00, 0x00}, []byte("webm")...)
	preview := engine.Preview(webmData, 10)
	if !strings.HasPrefix(preview, "WEBM ") {
		t.Errorf("Preview = %q, want prefix %q", preview, "WEBM ")
	}
}

func TestParseMP4DimensionsTruncated(t *testing.T) {
	// A truncated tkhd payload should not panic and should report no dims.
	if _, _, ok := parseTkhd([]byte{0, 0, 0}); ok {
		t.Error("parseTkhd on truncated data returned ok, want false")
	}
	// A moov box with no tkhd should yield no dimensions.
	moovOnly := buildBox("moov", buildBox("trak", buildBox("mdia", []byte{})))
	if _, _, ok := parseMP4Dimensions(moovOnly); ok {
		t.Error("parseMP4Dimensions on moov without tkhd returned ok, want false")
	}
}

func TestHumanReadableSize(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1 KB"},
		{1024 * 1024, "1 MB"},
		{150 * 1024 * 1024, "150 MB"},
		{1024 * 1024 * 1024, "1 GB"},
	}
	for _, tt := range tests {
		if got := humanReadableSize(tt.n); got != tt.want {
			t.Errorf("humanReadableSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
