package cli

import (
	"testing"
)

// TC-INIT-001: First initialization
func TestInit_FirstTime(t *testing.T) {
	h := NewTestHelper(t)

	output, err := h.RunInit()
	h.AssertNoError(err)
	h.AssertContains(output, "Drift project initialized")

	// Verify directory structure
	if !h.DirExists(".drift") {
		t.Error(".drift directory should exist")
	}
	if !h.DirExists(".drift/objects/blobs") {
		t.Error(".drift/objects/blobs should exist")
	}
	if !h.DirExists(".drift/objects/trees") {
		t.Error(".drift/objects/trees should exist")
	}
	if !h.DirExists(".drift/commits") {
		t.Error(".drift/commits should exist")
	}
	if !h.DirExists(".drift/refs") {
		t.Error(".drift/refs should exist")
	}
	// Note: .drift/index is lazily created on first SaveIndex call,
	// so it does not exist after init. LoadIndex handles this case.
}

// TC-INIT-002: Duplicate initialization
func TestInit_Duplicate(t *testing.T) {
	h := NewTestHelper(t)

	// First init
	_, err := h.RunInit()
	h.AssertNoError(err)

	// Second init should not error
	output, err := h.RunInit()
	h.AssertNoError(err)
	h.AssertContains(output, "Drift project already exists")
}

// TC-INIT-003: Command without initialization
func TestInit_UninitializedCommand(t *testing.T) {
	h := NewTestHelper(t)

	// Try status without init - should work (clean state)
	output, err := h.RunStatus()
	// Status works without init, just shows clean
	h.AssertNoError(err)
	h.AssertContains(output, "Nothing to commit, working tree clean")
}

// TC-INIT-004: Help works without initialization
func TestInit_HelpWithoutInit(t *testing.T) {
	h := NewTestHelper(t)

	// Help should work without init
	output, _ := CaptureOutput(func() error {
		return rootCmd.Help()
	})
	h.AssertContains(output, "Drift")
}

// TC-INIT-005: Untracked files after init (no commit)
func TestInit_UntrackedAfterInit(t *testing.T) {
	h := NewTestHelper(t)

	// Init project
	_, err := h.RunInit()
	h.AssertNoError(err)

	// Create a file
	h.WriteFile("note.txt", "new content")

	// Status should show untracked
	output, err := h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Untracked files:")
	h.AssertContains(output, "note.txt")
}
