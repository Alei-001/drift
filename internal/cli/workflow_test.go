package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TC-FLOW-001: Typical creative workflow
func TestFlow_TypicalCreativeWorkflow(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// 1. Create initial draft
	h.WriteFile("chapter1.txt", "第一章")
	h.WriteFile("chapter2.txt", "第二章")
	output, _ := h.RunAdd(".")
	h.AssertContains(output, "Added")
	output, _ = h.RunSave("初稿")
	h.AssertContains(output, "Saved version v1")

	// 2. Modify chapter 1
	h.WriteFile("chapter1.txt", "第一章 修改版")
	h.AddAndSave([]string{"chapter1.txt"}, "修改第一章")

	// 3. Create ending-a branch
	_, err := h.RunBranch("ending-a")
	h.AssertNoError(err)
	_, err = h.RunSwitch("ending-a")
	h.AssertNoError(err)
	h.WriteFile("ending.txt", "结局A")
	h.AddAndSave([]string{"ending.txt"}, "结局A")

	// 4. Create ending-b branch from main
	_, err = h.RunSwitch("main")
	h.AssertNoError(err)
	_, err = h.RunBranch("ending-b")
	h.AssertNoError(err)
	_, err = h.RunSwitch("ending-b")
	h.AssertNoError(err)
	h.WriteFile("ending.txt", "结局B")
	h.AddAndSave([]string{"ending.txt"}, "结局B")

	// 5. Verify branch independence
	output, _ = h.RunList()
	h.AssertContains(output, "v1")
	h.AssertContains(output, "修改第一章")
	h.AssertContains(output, "结局B")

	// 6. Switch to ending-a and verify
	_, err = h.RunSwitch("ending-a")
	h.AssertNoError(err)
	output, _ = h.RunList()
	h.AssertContains(output, "结局A")

	// 7. Export using branch name (main's v1)
	outputDir := filepath.Join(h.Dir, "export")
	output, err = h.RunExport("main", "-o", outputDir)
	h.AssertNoError(err)
	h.AssertContains(output, "Exported")

	// 8. Diff between branches
	output, _ = h.RunDiff("ending-a", "ending-b")
	h.AssertContains(output, "结局A")
	h.AssertContains(output, "结局B")
}

// TC-FLOW-002: Designer color scheme comparison
func TestFlow_DesignerColorScheme(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// 1. Create red theme on main branch
	h.WriteFile("theme.css", "color: red;")
	h.WriteFile("logo.png", string([]byte{0, 1, 2, 3, 0, 5})) // binary
	h.AddAndSave([]string{"theme.css", "logo.png"}, "红色主题")

	// 2. Create blue theme branch
	_, err := h.RunBranch("blue-theme")
	h.AssertNoError(err)
	_, err = h.RunSwitch("blue-theme")
	h.AssertNoError(err)
	h.WriteFile("theme.css", "color: blue;")
	h.AddAndSave([]string{"theme.css"}, "蓝色主题")

	// 3. Diff between branches (each branch has its own v1)
	output, _ := h.RunDiff("main", "blue-theme")
	h.AssertContains(output, "color: red")
	h.AssertContains(output, "color: blue")
}

// TC-FLOW-003: Restore workflow
func TestFlow_RestoreWorkflow(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create multiple versions
	h.WriteFile("draft.txt", "初稿内容")
	h.AddAndSave([]string{"draft.txt"}, "初稿")

	h.WriteFile("draft.txt", "修改稿内容")
	h.AddAndSave([]string{"draft.txt"}, "修改稿")

	h.WriteFile("draft.txt", "终稿内容")
	h.AddAndSave([]string{"draft.txt"}, "终稿")

	// Restore to v1
	output, err := h.RunRestore("v1")
	h.AssertNoError(err)
	h.AssertContains(output, "Restored to v1")

	// Verify content
	content := h.ReadFile("draft.txt")
	if content != "初稿内容" {
		t.Errorf("draft.txt = %q, want %q", content, "初稿内容")
	}

	// Restore updates index, so status shows staged changes
	output, _ = h.RunStatus()
	h.AssertContains(output, "Staged changes:")
}

// TC-FLOW-004: Branch and merge exploration
func TestFlow_BranchExploration(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create base version
	h.WriteFile("design.txt", "base design")
	h.AddAndSave([]string{"design.txt"}, "base")

	// Create variant A
	_, err := h.RunBranch("variant-a")
	h.AssertNoError(err)
	_, err = h.RunSwitch("variant-a")
	h.AssertNoError(err)
	h.WriteFile("design.txt", "variant A design")
	h.AddAndSave([]string{"design.txt"}, "variant A")

	// Create variant B from base
	_, err = h.RunSwitch("main")
	h.AssertNoError(err)
	_, err = h.RunBranch("variant-b")
	h.AssertNoError(err)
	_, err = h.RunSwitch("variant-b")
	h.AssertNoError(err)
	h.WriteFile("design.txt", "variant B design")
	h.AddAndSave([]string{"design.txt"}, "variant B")

	// Compare variants using branch names
	output, _ := h.RunDiff("variant-a", "variant-b")
	h.AssertContains(output, "variant A design")
	h.AssertContains(output, "variant B design")

	// List all branches
	output, _ = h.RunBranch("list")
	h.AssertContains(output, "main")
	h.AssertContains(output, "variant-a")
	h.AssertContains(output, "variant-b")
}

// TC-FLOW-005: Export and verify
func TestFlow_ExportAndVerify(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create version with multiple files
	h.WriteFile("src/main.go", "package main")
	h.WriteFile("src/utils.go", "package utils")
	h.WriteFile("README.md", "# Project")
	h.AddAndSave([]string{"src/main.go", "src/utils.go", "README.md"}, "initial")

	// Export to directory
	outputDir := filepath.Join(h.Dir, "release")
	output, err := h.RunExport("v1", "-o", outputDir)
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 3 file(s)")

	// Verify exported files
	if _, err := os.Stat(filepath.Join(outputDir, "src/main.go")); err != nil {
		t.Error("src/main.go should exist in export")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "src/utils.go")); err != nil {
		t.Error("src/utils.go should exist in export")
	}
	if _, err := os.Stat(filepath.Join(outputDir, "README.md")); err != nil {
		t.Error("README.md should exist in export")
	}

	// Export to zip
	zipPath := filepath.Join(h.Dir, "release.zip")
	output, err = h.RunExport("v1", "-o", zipPath, "-f", "zip")
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 3 file(s)")
	if _, err := os.Stat(zipPath); err != nil {
		t.Error("zip file should exist")
	}
}
