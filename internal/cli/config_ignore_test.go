package cli

import (
	"testing"
)

// TC-CONFIG-001: Default config
func TestConfig_DefaultConfig(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Verify config.json exists
	if !h.FileExists(".drift/config.json") {
		t.Error("config.json should exist after init")
	}
}

// TC-IGNORE-001: .driftignore ignores specific files
func TestIgnore_SpecificFiles(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create .driftignore
	h.WriteFile(".driftignore", "*.log")

	// Create files
	h.WriteFile("debug.log", "log content")
	h.WriteFile("note.txt", "real content")

	// Add all
	output, err := h.RunAdd(".")
	h.AssertNoError(err)
	h.AssertContains(output, "Added")

	// Status should only show note.txt
	output, err = h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "A note.txt")
	h.AssertNotContains(output, "debug.log")
}

// TC-IGNORE-002: .driftignore ignores directories
func TestIgnore_Directories(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create .driftignore
	h.WriteFile(".driftignore", "build/")

	// Create files
	h.WriteFile("build/out.txt", "build output")
	h.WriteFile("src.txt", "source")

	// Add all
	output, err := h.RunAdd(".")
	h.AssertNoError(err)
	h.AssertContains(output, "Added")

	// Status should only show src.txt
	output, err = h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "A src.txt")
	h.AssertNotContains(output, "build/out.txt")
}

// TC-IGNORE-003: .driftignore uses ** wildcard
func TestIgnore_DoubleStarWildcard(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create .driftignore with simpler ** pattern
	h.WriteFile(".driftignore", "node_modules/**")

	// Create files
	h.WriteFile("node_modules/pkg/index.js", "deps")
	h.WriteFile("app.js", "source")

	// Add all
	output, err := h.RunAdd(".")
	h.AssertNoError(err)
	h.AssertContains(output, "Added")

	// Status should only show app.js and .driftignore
	output, err = h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "A app.js")
	h.AssertNotContains(output, "node_modules")
}

// TC-IGNORE-004: .drift/ and .git/ always ignored
func TestIgnore_DriftAndGitIgnored(t *testing.T) {
	h := NewTestHelper(t)
	h.InitProject()

	// Create a file in .drift (should be ignored)
	h.WriteFile(".drift/test.txt", "should be ignored")

	// Add all
	output, err := h.RunAdd(".")
	h.AssertNoError(err)
	h.AssertContains(output, "Added")

	// Status should be clean (nothing staged)
	output, err = h.RunStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Nothing to commit, working tree clean")
}
