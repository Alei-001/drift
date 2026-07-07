package fsutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddIgnoreRules(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")

	added, err := AddIgnoreRules(filePath, []string{"*.tmp", "*.psd"})
	if err != nil {
		t.Fatalf("AddIgnoreRules failed: %v", err)
	}
	if len(added) != 2 {
		t.Errorf("expected 2 rules added, got %d", len(added))
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "*.tmp\n") {
		t.Errorf("file should contain '*.tmp'")
	}
	if !strings.Contains(content, "*.psd\n") {
		t.Errorf("file should contain '*.psd'")
	}
}

func TestAddIgnoreRules_Duplicate(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")

	if _, err := AddIgnoreRules(filePath, []string{"*.tmp"}); err != nil {
		t.Fatalf("first add failed: %v", err)
	}
	added, err := AddIgnoreRules(filePath, []string{"*.tmp", "*.psd"})
	if err != nil {
		t.Fatalf("second add failed: %v", err)
	}
	if len(added) != 1 {
		t.Errorf("expected 1 rule added, got %d", len(added))
	}

	rules, err := ListIgnoreRules(filePath)
	if err != nil {
		t.Fatalf("ListIgnoreRules failed: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("expected 2 rules total, got %d", len(rules))
	}

	count := 0
	for _, r := range rules {
		if r == "*.tmp" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected '*.tmp' to appear once, got %d", count)
	}
}

func TestAddIgnoreRules_PreservesComments(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")
	original := "# my ignores\n*.log\n"
	if err := os.WriteFile(filePath, []byte(original), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	added, err := AddIgnoreRules(filePath, []string{"*.tmp"})
	if err != nil {
		t.Fatalf("AddIgnoreRules failed: %v", err)
	}
	if len(added) != 1 {
		t.Errorf("expected 1 rule added, got %d", len(added))
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "# my ignores\n") {
		t.Errorf("comment should be preserved at top, got: %q", content)
	}
}

func TestListIgnoreRules(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")
	content := "# comment\n\n*.tmp\n*.psd\n\n# another comment\nbackup/\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rules, err := ListIgnoreRules(filePath)
	if err != nil {
		t.Fatalf("ListIgnoreRules failed: %v", err)
	}

	expected := []string{"*.tmp", "*.psd", "backup/"}
	if len(rules) != len(expected) {
		t.Fatalf("expected %d rules, got %d: %v", len(expected), len(rules), rules)
	}
	for i, r := range rules {
		if r != expected[i] {
			t.Errorf("rule %d: expected %q, got %q", i, expected[i], r)
		}
	}
}

func TestListIgnoreRules_NoFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")

	rules, err := ListIgnoreRules(filePath)
	if err != nil {
		t.Fatalf("ListIgnoreRules failed: %v", err)
	}
	if rules != nil {
		t.Errorf("expected nil rules for missing file, got %v", rules)
	}
}

func TestRemoveIgnoreRule(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")
	content := "*.tmp\n*.psd\nbackup/\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := RemoveIgnoreRule(filePath, "*.psd"); err != nil {
		t.Fatalf("RemoveIgnoreRule failed: %v", err)
	}

	rules, err := ListIgnoreRules(filePath)
	if err != nil {
		t.Fatalf("ListIgnoreRules failed: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules after removal, got %d", len(rules))
	}
	for _, r := range rules {
		if r == "*.psd" {
			t.Errorf("rule '*.psd' should have been removed")
		}
	}
}

func TestRemoveIgnoreRule_NotFound(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")
	content := "*.tmp\n*.psd\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	err := RemoveIgnoreRule(filePath, "*.log")
	if err == nil {
		t.Fatalf("expected error for non-existent rule")
	}
	if !strings.Contains(err.Error(), "*.log") {
		t.Errorf("error should mention the pattern, got: %v", err)
	}
}

func TestRemoveIgnoreRule_NoFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, ".driftignore")

	err := RemoveIgnoreRule(filePath, "*.tmp")
	if err == nil {
		t.Fatalf("expected error when file does not exist")
	}
}
