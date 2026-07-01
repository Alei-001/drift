package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalk_VisitsAllFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0644)

	var visited []string
	err := Walk(dir, ".driftignore", func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			visited = append(visited, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}
	if len(visited) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(visited), visited)
	}
}

func TestWalk_ExcludesDriftDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)
	os.MkdirAll(filepath.Join(dir, ".drift"), 0755)
	os.WriteFile(filepath.Join(dir, ".drift", "config.json"), []byte("{}"), 0644)

	var visited []string
	Walk(dir, ".driftignore", func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			visited = append(visited, rel)
		}
		return nil
	})

	for _, p := range visited {
		if filepath.HasPrefix(p, ".drift") {
			t.Errorf(".drift directory should be excluded, but visited: %s", p)
		}
	}
	if len(visited) != 1 {
		t.Errorf("expected 1 file (excluding .drift), got %d", len(visited))
	}
}

func TestWalk_RespectsIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".driftignore"), []byte("*.log\n"), 0644)
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.log"), []byte("skip"), 0644)

	var visited []string
	Walk(dir, ".driftignore", func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			visited = append(visited, rel)
		}
		return nil
	})

	for _, p := range visited {
		if p == "skip.log" {
			t.Error("skip.log should have been ignored")
		}
	}
	// .driftignore itself is also not visited
	found := false
	for _, p := range visited {
		if p == "keep.txt" {
			found = true
		}
	}
	if !found {
		t.Error("keep.txt should have been visited")
	}
}

func TestWalk_CustomIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".myignore"), []byte("*.tmp\n"), 0644)
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.tmp"), []byte("skip"), 0644)

	var visited []string
	Walk(dir, ".myignore", func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			visited = append(visited, rel)
		}
		return nil
	})

	for _, p := range visited {
		if p == "skip.tmp" {
			t.Error("skip.tmp should have been ignored by custom ignore file")
		}
	}
}

func TestWalk_BOMStripped(t *testing.T) {
	dir := t.TempDir()
	// Write .driftignore with UTF-8 BOM
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := append(bom, []byte("*.log\n")...)
	os.WriteFile(filepath.Join(dir, ".driftignore"), content, 0644)
	os.WriteFile(filepath.Join(dir, "skip.log"), []byte("skip"), 0644)
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)

	var visited []string
	Walk(dir, ".driftignore", func(path string, info os.FileInfo) error {
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			visited = append(visited, rel)
		}
		return nil
	})

	for _, p := range visited {
		if p == "skip.log" {
			t.Error("skip.log should have been ignored despite BOM in .driftignore")
		}
	}
}
