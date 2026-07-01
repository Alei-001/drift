package text

import (
	"bytes"
	"strings"
	"testing"
)

// --- Detect tests ---

func TestDetect_ByExtension(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		path string
		want bool
	}{
		{"readme.txt", true},
		{"notes.md", true},
		{"main.go", true},
		{"lib.rs", true},
		{"app.js", true},
		{"index.ts", true},
		{"script.py", true},
		{"Main.java", true},
		{"program.c", true},
		{"header.h", true},
		{"page.html", true},
		{"style.css", true},
		{"data.json", true},
		{"config.yaml", true},
		{"config.yml", true},
		{"setup.py", true},
		{"script.sh", true},
		{"run.bat", true},
		{"script.ps1", true},
		{"image.png", false},
		{"video.mp4", false},
		{"archive.zip", false},
		{"noext", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := engine.DetectByExtension(tc.path); got != tc.want {
				t.Errorf("DetectByExtension(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDetect_ByBasename(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		path string
		want bool
	}{
		{"Makefile", true},
		{"Dockerfile", true},
		{"LICENSE", true},
		{"README", true},
		{".gitignore", true},
		{".editorconfig", true},
		{".env", true},
		{"randomfile", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := engine.DetectByExtension(tc.path); got != tc.want {
				t.Errorf("DetectByExtension(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestDetect_ByMagic(t *testing.T) {
	engine := NewEngine()
	// Text has no magic bytes, so DetectByMagic always returns false.
	tests := []struct {
		name   string
		header []byte
	}{
		{"plain text", []byte("hello world")},
		{"empty", []byte{}},
		{"png magic", []byte{0x89, 'P', 'N', 'G'}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.DetectByMagic(tc.header); got != false {
				t.Errorf("DetectByMagic(%s) = %v, want false", tc.name, got)
			}
		})
	}
}

func TestDetect_ByHeuristic(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		name   string
		header []byte
		want   bool
	}{
		{"plain ascii", []byte("hello world"), true},
		{"utf-8 text", []byte("你好世界"), true},
		{"empty header", []byte{}, false},
		{"with null byte", []byte("hello\x00world"), false},
		{"binary data with null", []byte{0x00, 0x01, 0x02, 0x03}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := engine.DetectByHeuristic("testfile", tc.header); got != tc.want {
				t.Errorf("DetectByHeuristic(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// --- Diff tests ---

func TestDiff_Identical(t *testing.T) {
	engine := NewEngine()
	data := []byte("line1\nline2\nline3\n")

	diff, err := engine.Diff("old.txt", data, "new.txt", data)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for identical content, got %q", diff)
	}
}

func TestDiff_EmptyFiles(t *testing.T) {
	engine := NewEngine()

	diff, err := engine.Diff("old.txt", []byte{}, "new.txt", []byte{})
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for both empty, got %q", diff)
	}
}

func TestDiff_PureInsertions(t *testing.T) {
	engine := NewEngine()
	oldData := []byte("line1\nline2\n")
	newData := []byte("line1\nline2\nline3\nline4\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "+line3") {
		t.Errorf("expected diff to contain +line3, got %q", diff)
	}
	if !strings.Contains(diff, "+line4") {
		t.Errorf("expected diff to contain +line4, got %q", diff)
	}
	if strings.Contains(diff, "-line") {
		t.Errorf("expected no deletions, got %q", diff)
	}
}

func TestDiff_PureDeletions(t *testing.T) {
	engine := NewEngine()
	oldData := []byte("line1\nline2\nline3\nline4\n")
	newData := []byte("line1\nline2\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "-line3") {
		t.Errorf("expected diff to contain -line3, got %q", diff)
	}
	if !strings.Contains(diff, "-line4") {
		t.Errorf("expected diff to contain -line4, got %q", diff)
	}
	if strings.Count(diff, "+line") > 0 {
		t.Errorf("expected no insertions of 'line', got %q", diff)
	}
}

func TestDiff_ModifiedLine(t *testing.T) {
	engine := NewEngine()
	oldData := []byte("line1\nold line\nline3\n")
	newData := []byte("line1\nnew line\nline3\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "-old line") {
		t.Errorf("expected diff to contain -old line, got %q", diff)
	}
	if !strings.Contains(diff, "+new line") {
		t.Errorf("expected diff to contain +new line, got %q", diff)
	}
}

func TestDiff_CRLF_Normalized(t *testing.T) {
	engine := NewEngine()
	// Same content but different line endings — should produce empty diff
	lfData := []byte("line1\nline2\nline3\n")
	crlfData := []byte("line1\r\nline2\r\nline3\r\n")

	diff, err := engine.Diff("old.txt", lfData, "new.txt", crlfData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff for CRLF vs LF same content, got %q", diff)
	}
}

func TestDiff_CRLF_WithRealChanges(t *testing.T) {
	engine := NewEngine()
	oldData := []byte("line1\r\nold line\r\nline3\r\n")
	newData := []byte("line1\nnew line\nline3\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "-old line") {
		t.Errorf("expected diff to contain -old line, got %q", diff)
	}
	if !strings.Contains(diff, "+new line") {
		t.Errorf("expected diff to contain +new line, got %q", diff)
	}
}

func TestDiff_HunkMerge(t *testing.T) {
	engine := NewEngine()
	// Two changes separated by 5 lines — should be merged into one hunk
	// (contextSize=3, gap <= 2*contextSize=6 means merge)
	oldLines := []string{
		"line1", "line2", "line3", "line4", "line5",
		"oldA", "line7", "line8", "line9", "line10", "line11",
		"oldB", "line13",
	}
	newLines := []string{
		"line1", "line2", "line3", "line4", "line5",
		"newA", "line7", "line8", "line9", "line10", "line11",
		"newB", "line13",
	}
	oldData := []byte(strings.Join(oldLines, "\n") + "\n")
	newData := []byte(strings.Join(newLines, "\n") + "\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	// Count hunk headers (@@ ... @@)
	hunkCount := strings.Count(diff, "@@ -")
	if hunkCount != 1 {
		t.Errorf("expected 1 merged hunk, got %d hunks. diff:\n%s", hunkCount, diff)
	}
}

func TestDiff_SeparateHunks(t *testing.T) {
	engine := NewEngine()
	// Two changes separated by many lines — should be separate hunks
	var oldLines, newLines []string
	oldLines = append(oldLines, "header")
	newLines = append(newLines, "header-modified")
	for i := 0; i < 20; i++ {
		oldLines = append(oldLines, "line")
		newLines = append(newLines, "line")
	}
	oldLines = append(oldLines, "footer")
	newLines = append(newLines, "footer-modified")

	oldData := []byte(strings.Join(oldLines, "\n") + "\n")
	newData := []byte(strings.Join(newLines, "\n") + "\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	hunkCount := strings.Count(diff, "@@ -")
	if hunkCount < 2 {
		t.Errorf("expected at least 2 hunks, got %d. diff:\n%s", hunkCount, diff)
	}
}

func TestDiff_LargeFile(t *testing.T) {
	engine := NewEngine()
	// Build two 5000-line files with a small number of changes.
	// Myers algorithm should handle this quickly without OOM.
	var oldLines, newLines []string
	for i := 0; i < 5000; i++ {
		line := "line number " + itoa(i)
		oldLines = append(oldLines, line)
		if i == 100 {
			newLines = append(newLines, "modified line 100")
		} else if i == 2000 {
			newLines = append(newLines, "modified line 2000")
		} else {
			newLines = append(newLines, line)
		}
	}
	// Insert a few lines in the middle
	for i := 0; i < 5; i++ {
		newLines = append(newLines[:3001], append([]string{"inserted " + itoa(i)}, newLines[3001:]...)...)
	}

	oldData := []byte(strings.Join(oldLines, "\n") + "\n")
	newData := []byte(strings.Join(newLines, "\n") + "\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed on large file: %v", err)
	}
	if diff == "" {
		t.Error("expected non-empty diff for modified large file")
	}
	if !strings.Contains(diff, "modified line 100") {
		t.Error("expected diff to contain modified line 100")
	}
	if !strings.Contains(diff, "modified line 2000") {
		t.Error("expected diff to contain modified line 2000")
	}
	if !strings.Contains(diff, "inserted 0") {
		t.Error("expected diff to contain inserted lines")
	}
}

func TestDiff_AllNewFile(t *testing.T) {
	engine := NewEngine()
	oldData := []byte{}
	newData := []byte("line1\nline2\nline3\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "--- old.txt") {
		t.Errorf("expected diff header with old path, got %q", diff)
	}
	if !strings.Contains(diff, "+++ new.txt") {
		t.Errorf("expected diff header with new path, got %q", diff)
	}
	if strings.Count(diff, "+line") != 3 {
		t.Errorf("expected 3 inserted lines, got %q", diff)
	}
}

func TestDiff_AllDeleted(t *testing.T) {
	engine := NewEngine()
	oldData := []byte("line1\nline2\nline3\n")
	newData := []byte{}

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if strings.Count(diff, "-line") != 3 {
		t.Errorf("expected 3 deleted lines, got %q", diff)
	}
}

func TestDiff_UnifiedFormatHeader(t *testing.T) {
	engine := NewEngine()
	oldData := []byte("a\n")
	newData := []byte("b\n")

	diff, err := engine.Diff("old.txt", oldData, "new.txt", newData)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	lines := strings.SplitN(diff, "\n", 3)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines in diff, got %q", diff)
	}
	if lines[0] != "--- old.txt" {
		t.Errorf("expected '--- old.txt', got %q", lines[0])
	}
	if lines[1] != "+++ new.txt" {
		t.Errorf("expected '+++ new.txt', got %q", lines[1])
	}
}

// --- Preview tests ---

func TestPreview_ShortFile(t *testing.T) {
	engine := NewEngine()
	data := []byte("line1\nline2\nline3\n")
	preview := engine.Preview(data, 10)

	previewLines := strings.Split(preview, "\n")
	if len(previewLines) != 3 {
		t.Errorf("expected 3 lines in preview, got %d. preview:\n%s", len(previewLines), preview)
	}
	if !strings.Contains(preview, "line1") {
		t.Errorf("preview should contain line1, got %q", preview)
	}
	if !strings.Contains(preview, "line3") {
		t.Errorf("preview should contain line3, got %q", preview)
	}
}

func TestPreview_Truncated(t *testing.T) {
	engine := NewEngine()
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line "+itoa(i))
	}
	data := []byte(strings.Join(lines, "\n") + "\n")
	preview := engine.Preview(data, 5)

	// Preview should return only first 5 lines
	previewLines := strings.Split(preview, "\n")
	if len(previewLines) != 5 {
		t.Errorf("expected 5 lines in preview, got %d. preview:\n%s", len(previewLines), preview)
	}
	if !strings.Contains(preview, "line 0") {
		t.Errorf("preview should contain first line, got %q", preview)
	}
	if strings.Contains(preview, "line 5") {
		t.Errorf("preview should not contain line 5, got %q", preview)
	}
}

func TestPreview_EmptyFile(t *testing.T) {
	engine := NewEngine()
	preview := engine.Preview([]byte{}, 10)
	if preview != "" {
		t.Errorf("expected empty preview for empty file, got %q", preview)
	}
}

// --- Name and NewEngine ---

func TestName(t *testing.T) {
	engine := NewEngine()
	if got := engine.Name(); got != "text" {
		t.Errorf("Name() = %q, want %q", got, "text")
	}
}

// --- ChunkerFor smoke test ---

func TestChunkerFor_SmallFile(t *testing.T) {
	engine := NewEngine()
	// < 64KB should return nil (whole-file single chunk)
	c := engine.ChunkerFor(10 * 1024, nil)
	if c != nil {
		t.Errorf("expected nil chunker for small text file (whole-file), got %v", c)
	}
}

func TestChunkerFor_MediumFile(t *testing.T) {
	engine := NewEngine()
	// 64K-50MB should use FastCDC
	c := engine.ChunkerFor(200 * 1024, nil)
	if c == nil {
		t.Fatal("expected non-nil chunker for 200KB text file, got nil")
	}
	data := bytes.Repeat([]byte("text content line here\n"), 10000)
	chunks, err := c.Chunk(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Chunk failed: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk, got none")
	}
	var total uint32
	for _, ch := range chunks {
		total += ch.Size
	}
	if int(total) != len(data) {
		t.Errorf("total chunk size %d != original %d", total, len(data))
	}
}

// --- helpers ---

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
