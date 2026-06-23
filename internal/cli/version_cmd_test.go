package cli

import "testing"

// TC-VER-001: version command outputs version string
func TestVersion_Output(t *testing.T) {
	h := NewTestHelper(t)
	// No InitProject — version must work outside a drift project.

	saved := version
	defer func() { version = saved }()
	version = "1.2.3"

	output, err := CaptureOutput(func() error {
		return versionCmd.RunE(versionCmd, nil)
	})
	h.AssertNoError(err)
	h.AssertContains(output, "drift 1.2.3")
}

// TC-VER-002: version works without a drift project (no PersistentPreRunE error)
func TestVersion_WorksWithoutInit(t *testing.T) {
	h := NewTestHelper(t)
	// Do NOT call InitProject — version should not require .drift/.
	h.SetupSharedState()

	output, err := CaptureOutput(func() error {
		return versionCmd.RunE(versionCmd, nil)
	})
	h.AssertNoError(err)
	if output == "" {
		t.Error("expected version output, got empty string")
	}
}
