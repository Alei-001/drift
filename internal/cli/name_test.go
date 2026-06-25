package cli

import (
	"testing"
)

// TC-NAME-001: Assign a name to a version
func TestName_Add(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	id1 := h.AddAndSave([]string{"a.txt"}, "v1")

	output, err := h.RunName(id1, "final")
	h.AssertNoError(err)
	h.AssertContains(output, "Named "+id1)
	h.AssertContains(output, "final")
}

// TC-NAME-002: List names
func TestName_List(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	id1 := h.AddAndSave([]string{"a.txt"}, "v1")

	_, err := h.RunName(id1, "draft")
	h.AssertNoError(err)

	output, err := h.RunName("--list")
	h.AssertNoError(err)
	h.AssertContains(output, "draft")
	h.AssertContains(output, "v1")
}

// TC-NAME-003: List with no names defined
func TestName_ListEmpty(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.RunName("--list")
	h.AssertNoError(err)
	h.AssertContains(output, "No version names defined")
}

// TC-NAME-004: Delete a name
func TestName_Delete(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	id1 := h.AddAndSave([]string{"a.txt"}, "v1")

	_, err := h.RunName(id1, "temp")
	h.AssertNoError(err)

	output, err := h.RunName("--delete=temp")
	h.AssertNoError(err)
	h.AssertContains(output, "Deleted name 'temp'")

	// Verify it's gone
	output, err = h.RunName("--list")
	h.AssertNoError(err)
	h.AssertContains(output, "No version names defined")
}

// TC-NAME-005: Delete nonexistent name errors
func TestName_DeleteNotFound(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunName("--delete=nonexistent")
	h.AssertError(err)
}

// TC-NAME-006: Name with invalid label errors
func TestName_InvalidLabel(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	id1 := h.AddAndSave([]string{"a.txt"}, "v1")

	// Path separator in label
	_, err := h.RunName(id1, "bad/name")
	h.AssertError(err)

	// Empty label
	_, err = h.RunName(id1, "")
	h.AssertError(err)
}

// TC-NAME-007: Name with nonexistent version errors
func TestName_VersionNotFound(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunName("v99", "label")
	h.AssertError(err)
}

// TC-NAME-008: No arguments without flags errors
func TestName_NoArgs(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.RunName()
	h.AssertError(err)
}

// TC-NAME-009: Resolve version by name
func TestName_ResolveByName(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1")
	id1 := h.AddAndSave([]string{"a.txt"}, "v1")

	// Assign a name
	_, err := h.RunName(id1, "milestone")
	h.AssertNoError(err)

	// Use the name to export (resolveCommit should find it)
	outputDir := h.Dir + "/output"
	output, err := h.RunExport("milestone", "-o", outputDir)
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 1 file(s)")
}

// TC-NAME-010: Overwrite existing name
func TestName_Overwrite(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1")
	id1 := h.AddAndSave([]string{"a.txt"}, "v1")

	h.WriteFile("a.txt", "v2")
	id2 := h.AddAndSave([]string{"a.txt"}, "v2")

	// Assign name to v1
	_, err := h.RunName(id1, "label")
	h.AssertNoError(err)

	// Reassign to v2
	_, err = h.RunName(id2, "label")
	h.AssertNoError(err)

	// Verify it now points to v2
	output, err := h.RunName("--list")
	h.AssertNoError(err)
	h.AssertContains(output, "v2")
}

// TC-NAME-011: save --name assigns alias at save time
func TestSave_WithName(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)

	output, err := h.RunSaveWithName("my version", "final")
	h.AssertNoError(err)
	id1 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id1)
	h.AssertContains(output, "final")

	// Verify the name was assigned
	output, err = h.RunName("--list")
	h.AssertNoError(err)
	h.AssertContains(output, "final")
	h.AssertContains(output, id1)
}

// TC-NAME-012: save --name with invalid label fails before saving
func TestSave_WithInvalidName(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)

	// Invalid label should fail before saving
	_, err = h.RunSaveWithName("msg", "bad/name")
	h.AssertError(err)

	// Verify nothing was saved
	_, err = h.RunName("--list")
	h.AssertNoError(err)
}

// TC-NAME-013: save --name can be resolved later
func TestSave_WithNameResolved(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "content")
	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)

	_, err = h.RunSaveWithName("msg", "milestone")
	h.AssertNoError(err)

	// Use the name to export
	outputDir := h.Dir + "/output"
	output, err := h.RunExport("milestone", "-o", outputDir)
	h.AssertNoError(err)
	h.AssertContains(output, "Exported 1 file(s)")
}

// TC-NAME-014: save without --name does not affect existing names
func TestSave_WithoutName(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	h.WriteFile("a.txt", "v1")
	_, err := h.RunAdd("a.txt")
	h.AssertNoError(err)
	_, err = h.RunSaveWithName("v1", "first")
	h.AssertNoError(err)

	// Save again without --name
	h.WriteFile("a.txt", "v2")
	_, err = h.RunAdd("a.txt")
	h.AssertNoError(err)
	output, err := h.RunSave("v2")
	h.AssertNoError(err)
	id2 := h.ExtractSaveID(output)
	h.AssertContains(output, "Saved version "+id2)

	// Verify "first" still points to v1
	output, err = h.RunName("--list")
	h.AssertNoError(err)
	h.AssertContains(output, "first")
	h.AssertContains(output, "v1")
}
