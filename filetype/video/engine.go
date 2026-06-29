package video

import (
	"bytes"
	"path/filepath"
	"strings"
)

// videoExtensions maps supported video file extensions.
var videoExtensions = map[string]bool{
	".mp4":  true,
	".mov":  true,
	".avi":  true,
	".mkv":  true,
	".webm": true,
}

// VideoEngine handles common video container formats (MP4, MOV, AVI, MKV, WebM).
// Detection is purely byte-based; no third-party codecs are used.
type VideoEngine struct{}

// NewEngine creates a new VideoEngine.
func NewEngine() *VideoEngine {
	return &VideoEngine{}
}

// Name returns "video".
func (e *VideoEngine) Name() string {
	return "video"
}

// DetectByMagic checks file header signatures for known video container
// magic bytes.
func (e *VideoEngine) DetectByMagic(header []byte) bool {
	return detectVideoMagic(header)
}

// DetectByExtension checks if the path's extension is a known video type.
func (e *VideoEngine) DetectByExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return videoExtensions[ext]
}

// DetectByHeuristic returns false; videos are not sniffed heuristically.
func (e *VideoEngine) DetectByHeuristic(path string, header []byte) bool {
	return false
}

// detectVideoMagic checks the header for known video container signatures.
func detectVideoMagic(data []byte) bool {
	return detectVideoFormat(data) != ""
}

// detectVideoFormat identifies the container format from magic bytes.
// Returns "MP4" for MP4/MOV, "AVI" for AVI, "MKV"/"WEBM" for EBML containers,
// or "" if no known signature is found.
func detectVideoFormat(data []byte) string {
	// MP4/MOV: "ftyp" at offset 4.
	if len(data) >= 8 && bytes.Equal(data[4:8], []byte("ftyp")) {
		return "MP4"
	}
	// AVI: "RIFF"...."AVI ".
	if len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("AVI ")) {
		return "AVI"
	}
	// MKV/WebM: EBML header magic.
	if len(data) >= 4 && data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		return detectEBMLDocType(data)
	}
	return ""
}

// detectEBMLDocType distinguishes WebM from Matroska by scanning for the
// DocType string in the EBML header. Falls back to "MKV" if the DocType
// cannot be determined.
func detectEBMLDocType(data []byte) string {
	// The DocType string lives near the start of the EBML header. A simple
	// substring scan is robust and avoids implementing the full VINT parser.
	end := len(data)
	if end > 64 {
		end = 64
	}
	head := data[:end]
	if bytes.Contains(head, []byte("webm")) {
		return "WEBM"
	}
	if bytes.Contains(head, []byte("matroska")) {
		return "MKV"
	}
	return "MKV"
}
