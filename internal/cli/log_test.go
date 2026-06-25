package cli

import (
	"strings"
	"testing"
)

// TC-LOG-001: log shows recent operations with default limit
func TestLog_DefaultLimit(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create 3 saves → 3 operations.
	for i := 1; i <= 3; i++ {
		h.WriteFile("f.txt", "content"+string(rune('0'+i)))
		h.AddAndSave([]string{"f.txt"}, "")
	}

	output, err := h.RunLog(20) // default
	h.AssertNoError(err)
	h.AssertContains(output, "Recent operations")
	// Should list 3 numbered entries.
	if got := strings.Count(output, "save"); got < 3 {
		t.Errorf("expected at least 3 'save' occurrences, got %d\noutput:\n%s", got, output)
	}
}

// TC-LOG-002: log -n limits the number of entries shown
func TestLog_LimitN(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	for i := 1; i <= 5; i++ {
		h.WriteFile("f.txt", "content"+string(rune('0'+i)))
		h.AddAndSave([]string{"f.txt"}, "")
	}

	output, err := h.RunLog(2)
	h.AssertNoError(err)
	// Count numbered list entries "  N. " — should be exactly 2.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	numbered := 0
	for _, line := range lines {
		if strings.Contains(line, ". ") && (strings.Contains(line, "save") || strings.Contains(line, "switch")) {
			numbered++
		}
	}
	if numbered != 2 {
		t.Errorf("expected 2 numbered entries with -n 2, got %d\noutput:\n%s", numbered, output)
	}
	h.AssertContains(output, "more older operations")
}

// TC-LOG-003: log -n 0 shows all entries
func TestLog_ShowAll(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	for i := 1; i <= 25; i++ {
		h.WriteFile("f.txt", "content"+string(rune('0'+i)))
		h.AddAndSave([]string{"f.txt"}, "")
	}

	output, err := h.RunLog(0)
	h.AssertNoError(err)
	// With -n 0, no "more older operations" hint should appear.
	h.AssertNotContains(output, "more older operations")
}

// TC-LOG-004: log with no operations
func TestLog_Empty(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunLog(20)
	h.AssertNoError(err)
	h.AssertContains(output, "No operations recorded yet")
}
