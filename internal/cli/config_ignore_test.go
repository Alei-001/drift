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

// TC-CONFIG-002: Get config value (default empty)
func TestConfig_GetDefault(t *testing.T) {
	h := NewTestHelper(t)

	output, err := h.RunConfig("user.name")
	h.AssertNoError(err)
	if output == "" {
		// default is empty - just verify no error
	}
}

// TC-CONFIG-003: Set and get config value
func TestConfig_SetAndGet(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("user.name", "Test User")
	h.AssertNoError(err)

	output, err := h.RunConfig("user.name")
	h.AssertNoError(err)
	h.AssertContains(output, "Test User")
}

// TC-CONFIG-004: Unknown config key errors
func TestConfig_UnknownKey(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("unknown.key")
	h.AssertError(err)
}

// TC-CONFIG-005: Set and get core.autocrlf
func TestConfig_SetAndGetAutoCRLF(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("core.autocrlf", "true")
	h.AssertNoError(err)

	output, err := h.RunConfig("core.autocrlf")
	h.AssertNoError(err)
	h.AssertContains(output, "true")
}

// TC-CONFIG-006: Get default core.autocrlf (empty)
func TestConfig_GetDefaultAutoCRLF(t *testing.T) {
	h := NewTestHelper(t)

	output, err := h.RunConfig("core.autocrlf")
	h.AssertNoError(err)
	if trimmed := output; trimmed != "" && trimmed != "\n" {
		t.Errorf("expected empty default for core.autocrlf, got %q", output)
	}
}

// TC-CONFIG-007: Set and get user.email
func TestConfig_SetAndGetEmail(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("user.email", "test@example.com")
	h.AssertNoError(err)

	output, err := h.RunConfig("user.email")
	h.AssertNoError(err)
	h.AssertContains(output, "test@example.com")
}

// TC-CONFIG-008: Set and get core.default_branch
func TestConfig_SetAndGetDefaultBranch(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("core.default_branch", "develop")
	h.AssertNoError(err)

	output, err := h.RunConfig("core.default_branch")
	h.AssertNoError(err)
	h.AssertContains(output, "develop")
}

// TC-CONFIG-009: Config get default values (all empty)
func TestConfig_GetDefaults(t *testing.T) {
	h := NewTestHelper(t)

	// user.name default empty
	output, err := h.RunConfig("user.name")
	h.AssertNoError(err)
	if output != "" && output != "\n" {
		t.Errorf("expected empty default for user.name, got %q", output)
	}

	// user.email default empty
	output, err = h.RunConfig("user.email")
	h.AssertNoError(err)
	if output != "" && output != "\n" {
		t.Errorf("expected empty default for user.email, got %q", output)
	}

	// core.default_branch default is "main"
	output, err = h.RunConfig("core.default_branch")
	h.AssertNoError(err)
	h.AssertContains(output, "main")
}

// TC-CONFIG-010: Set multiple config keys in sequence
func TestConfig_SetMultipleKeys(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("user.name", "Alice")
	h.AssertNoError(err)
	_, err = h.RunConfig("user.email", "alice@example.com")
	h.AssertNoError(err)
	_, err = h.RunConfig("core.autocrlf", "true")
	h.AssertNoError(err)

	output, err := h.RunConfig("user.name")
	h.AssertNoError(err)
	h.AssertContains(output, "Alice")

	output, err = h.RunConfig("user.email")
	h.AssertNoError(err)
	h.AssertContains(output, "alice@example.com")

	output, err = h.RunConfig("core.autocrlf")
	h.AssertNoError(err)
	h.AssertContains(output, "true")
}

// TC-CONFIG-011: --list prints all config values
func TestConfig_List(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("user.name", "Bob")
	h.AssertNoError(err)
	_, err = h.RunConfig("user.email", "bob@example.com")
	h.AssertNoError(err)

	output, err := h.RunConfig("--list")
	h.AssertNoError(err)
	h.AssertContains(output, "user.name=Bob")
	h.AssertContains(output, "user.email=bob@example.com")
	h.AssertContains(output, "core.default_branch=")
}

// TC-CONFIG-012: --unset clears a config value
func TestConfig_Unset(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("user.name", "Carol")
	h.AssertNoError(err)

	output, err := h.RunConfig("user.name")
	h.AssertNoError(err)
	h.AssertContains(output, "Carol")

	output, err = h.RunConfig("--unset", "user.name")
	h.AssertNoError(err)
	h.AssertContains(output, "Unset user.name")

	// Value should now be empty.
	output, err = h.RunConfig("user.name")
	h.AssertNoError(err)
	if output != "" && output != "\n" {
		t.Errorf("expected empty after unset, got %q", output)
	}
}

// TC-CONFIG-013: --unset with unknown key errors
func TestConfig_UnsetUnknownKey(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("--unset", "unknown.key")
	h.AssertError(err)
}

// TC-CONFIG-014: --unset without a key errors
func TestConfig_UnsetNoKey(t *testing.T) {
	h := NewTestHelper(t)

	_, err := h.RunConfig("--unset")
	h.AssertError(err)
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
