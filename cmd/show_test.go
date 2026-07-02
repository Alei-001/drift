package cmd

import (
	"testing"
)

func TestSafePreviewExt(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantExt string
	}{
		{"executable sanitized to bin", "payload.exe", ".bin"},
		{"bat sanitized to bin", "script.bat", ".bin"},
		{"ps1 sanitized to bin", "script.ps1", ".bin"},
		{"sh sanitized to bin", "script.sh", ".bin"},
		{"com sanitized to bin", "legacy.com", ".bin"},
		{"scr sanitized to bin", "screensaver.scr", ".bin"},
		{"txt preserved", "notes.txt", ".txt"},
		{"md preserved", "readme.md", ".md"},
		{"png preserved", "image.png", ".png"},
		{"jpg preserved", "image.jpg", ".jpg"},
		{"jpeg preserved", "image.jpeg", ".jpeg"},
		{"gif preserved", "anim.gif", ".gif"},
		{"webp preserved", "image.webp", ".webp"},
		{"bmp preserved", "image.bmp", ".bmp"},
		{"pdf preserved", "doc.pdf", ".pdf"},
		{"csv preserved", "data.csv", ".csv"},
		{"json preserved", "data.json", ".json"},
		{"xml preserved", "data.xml", ".xml"},
		{"html preserved", "page.html", ".html"},
		{"uppercase exe sanitized", "PAYLOAD.EXE", ".bin"},
		{"mixed case exe sanitized", "Payload.Exe", ".bin"},
		{"uppercase PNG preserved", "image.PNG", ".png"},
		{"no extension becomes bin", "README", ".bin"},
		{"path with exe sanitized", "dir/payload.exe", ".bin"},
		{"path with txt preserved", "dir/notes.txt", ".txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safePreviewExt(tt.path); got != tt.wantExt {
				t.Errorf("safePreviewExt(%q) = %q, want %q", tt.path, got, tt.wantExt)
			}
		})
	}
}
