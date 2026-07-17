package video

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Alei-001/drift/internal/chunker"
	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/util/format"
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
// Uses coarse-grained FastCDC (512K–4M) because video files are large
// and individual frames are not independently useful for CD dedup.
type VideoEngine struct{}

const videoChunkMinSize = 512 * 1024    // 512 KB
const videoChunkAvgSize = 1 * 1024 * 1024 // 1 MB
const videoChunkMaxSize = 4 * 1024 * 1024  // 4 MB

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

// ChunkerFor returns a coarse-grained FastCDC chunker (512K min, 1M avg,
// 4M max) tuned for video files. Video containers compress poorly with
// standard CD chunking because re-encodes change every byte, so larger
// chunks reduce metadata overhead without sacrificing the dedup guarantee.
func (e *VideoEngine) ChunkerFor(fileSize int64) chunker.Chunker {
	return chunker.NewFastCDCChunkerWithParams(videoChunkMinSize, videoChunkAvgSize, videoChunkMaxSize)
}

// Metadata returns the file metadata for video files.
func (e *VideoEngine) Metadata() *core.FileMetadata {
	return &core.FileMetadata{MIMEType: "application/octet-stream"}
}

// Diff compares two video files by size only, streaming both readers to count
// bytes rather than buffering either file in memory.
func (e *VideoEngine) Diff(ctx context.Context, oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
	oldSize, err := io.Copy(io.Discard, oldReader)
	if err != nil {
		return "", fmt.Errorf("read old video %s: %w", oldPath, err)
	}
	newSize, err := io.Copy(io.Discard, newReader)
	if err != nil {
		return "", fmt.Errorf("read new video %s: %w", newPath, err)
	}
	if oldSize == newSize {
		return "", nil
	}
	return fmt.Sprintf("video file size changed: %s -> %s",
		format.Bytes(oldSize), format.Bytes(newSize)), nil
}
