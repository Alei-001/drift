package filetype_test

import (
	"testing"

	"github.com/Alei-001/drift/internal/filetype"
	"github.com/Alei-001/drift/internal/filetype/binary"
	"github.com/Alei-001/drift/internal/filetype/image"
	"github.com/Alei-001/drift/internal/filetype/text"
	"github.com/Alei-001/drift/internal/filetype/video"
)

// Compile-time assertions that every engine satisfies the filetype.Engine
// interface. Lives in the external test package to avoid import cycles
// (sub-package tests cannot import filetype once init.go registers them).
var (
	_ filetype.Engine = (*text.TextEngine)(nil)
	_ filetype.Engine = (*image.ImageEngine)(nil)
	_ filetype.Engine = (*video.VideoEngine)(nil)
	_ filetype.Engine = (*binary.BinaryEngine)(nil)
)

// TestRegistrationOrder verifies the registration order text → image → video
// → binary so that specific formats are matched before the binary fallback.
func TestRegistrationOrder(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		header      []byte
		wantEngine  string
	}{
		{
			name:       "png matches image engine not binary",
			path:       "photo.png",
			header:     []byte("\x89PNG\r\n\x1a\n"),
			wantEngine: "image",
		},
		{
			name:       "jpg matches image engine not binary",
			path:       "photo.jpg",
			header:     []byte("\xFF\xD8\xFF\xE0"),
			wantEngine: "image",
		},
		{
			name:       "mp4 matches video engine not binary",
			path:       "clip.mp4",
			header:     []byte("\x00\x00\x00\x18ftypmp4\x00\x00\x00\x00"),
			wantEngine: "video",
		},
		{
			name:       "mkv matches video engine not binary",
			path:       "movie.mkv",
			header:     []byte("\x1A\x45\xDF\xA3"),
			wantEngine: "video",
		},
		{
			name:       "txt matches text engine",
			path:       "readme.txt",
			header:     []byte("hello world"),
			wantEngine: "text",
		},
		{
			name:       "unknown extension falls back to binary",
			path:       "data.dat",
			header:     []byte{0x00, 0x01, 0x02, 0x03},
			wantEngine: "binary",
		},
		{
			name:       "unknown binary with null bytes falls back to binary",
			path:       "noext",
			header:     []byte("foo\x00bar\x00"),
			wantEngine: "binary",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := filetype.DetectEngine(tc.path, tc.header)
			if e == nil {
				t.Fatalf("DetectEngine returned nil for %q", tc.path)
			}
			if got := e.Name(); got != tc.wantEngine {
				t.Fatalf("engine name = %q, want %q (path=%q)", got, tc.wantEngine, tc.path)
			}
		})
	}
}
