package cli

import (
	"regexp"
	"strings"
	"testing"
)

// TC-SAVE-001: Save without message
func TestSave_WithoutMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	_, err := h.RunAdd("note.txt")
	h.AssertNoError(err)

	output, err := h.RunSave("")
	h.AssertNoError(err)
	h.AssertContains(output, "Saved version v1")

	// Verify commit exists
	if h.VersionCount() != 1 {
		t.Errorf("expected 1 commit, got %d", h.VersionCount())
	}
}

// TC-SAVE-002: Save with message
func TestSave_WithMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("ch1.txt", "chapter 1")
	_, err := h.RunAdd("ch1.txt")
	h.AssertNoError(err)

	output, err := h.RunSave("first chapter")
	h.AssertNoError(err)
	h.AssertContains(output, "Saved version v1: first chapter")
}

// TC-SAVE-003: Save with empty staging area
func TestSave_EmptyStaging(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunSave("")
	h.AssertError(err)
}

// TC-SAVE-004: Version number auto-increment
func TestSave_VersionIncrement(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Save v1
	h.WriteFile("f1.txt", "v1")
	h.AddAndSave([]string{"f1.txt"}, "v1")

	// Save v2
	h.WriteFile("f2.txt", "v2")
	h.AddAndSave([]string{"f2.txt"}, "v2")

	// Save v3
	h.WriteFile("f3.txt", "v3")
	h.AddAndSave([]string{"f3.txt"}, "v3")

	if h.VersionCount() != 3 {
		t.Errorf("expected 3 commits, got %d", h.VersionCount())
	}
}

// TC-SAVE-005: Status clean after save
func TestSave_StatusCleanAfterSave(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "v1")

	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Nothing to commit, working tree clean")
}

// TC-LIST-001: Show version history
func TestList_ShowHistory(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create 3 versions
	h.WriteFile("f1.txt", "v1")
	h.AddAndSave([]string{"f1.txt"}, "v1")

	h.WriteFile("f2.txt", "v2")
	h.AddAndSave([]string{"f2.txt"}, "v2")

	h.WriteFile("f3.txt", "v3")
	h.AddAndSave([]string{"f3.txt"}, "v3")

	output, err := h.RunList()
	h.AssertNoError(err)
	h.AssertContains(output, "Version history:")
	h.AssertContains(output, "v3")
	h.AssertContains(output, "v2")
	h.AssertContains(output, "v1")
}

// TC-LIST-002: No versions yet
func TestList_NoVersions(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunList()
	h.AssertNoError(err)
	h.AssertContains(output, "No versions yet")
}

// TC-LIST-003: Version without message
func TestList_NoMessage(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("note.txt", "content")
	h.AddAndSave([]string{"note.txt"}, "")

	output, err := h.RunList()
	h.AssertNoError(err)
	h.AssertContains(output, "v1")
	// When message is empty, the line should end with the timestamp (no trailing message).
	// Format: "  v1  2026-06-22 22:10  <message>"
	// Use regex to extract the message part after the timestamp.
	re := regexp.MustCompile(`v\d+\s+\d{4}-\d{2}-\d{2} \d{2}:\d{2}\s*(.*)`)
	for _, line := range strings.Split(output, "\n") {
		m := re.FindStringSubmatch(line)
		if m != nil {
			msg := strings.TrimSpace(m[1])
			if msg != "" {
				t.Errorf("expected empty message, got %q in line %q", msg, line)
			}
		}
	}
}
