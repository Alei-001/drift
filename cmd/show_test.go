package cmd

import (
	"errors"
	"strings"
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

func TestShow_ValidSnapshot(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "file.txt", "hello world", "initial")

	out := captureStdout(t, func() {
		if err := runCmd(showCmd, []string{"head"}); err != nil {
			t.Fatalf("show: %v", err)
		}
	})

	if !strings.Contains(out, "file.txt") {
		t.Errorf("stdout = %q, want 'file.txt'", out)
	}
	if !strings.Contains(out, "Snapshot") {
		t.Errorf("stdout = %q, want 'Snapshot'", out)
	}
}

func TestShow_InvalidSnapshot(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(showCmd, []string{"id:nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "snapshot not found") {
		t.Errorf("stderr = %q, want 'snapshot not found'", strings.TrimSpace(errOut))
	}
}

func TestShow_FileContent(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "file.txt", "line1\nline2\n", "initial")

	out := captureStdout(t, func() {
		if err := runCmd(showCmd, []string{"head", "file.txt"}); err != nil {
			t.Fatalf("show file: %v", err)
		}
	})

	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Errorf("stdout = %q, want file content 'line1' and 'line2'", out)
	}
}

func TestShow_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(showCmd, []string{"head"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not a drift repository") {
		t.Errorf("stderr = %q, want 'not a drift repository'", strings.TrimSpace(errOut))
	}
}
