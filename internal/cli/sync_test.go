package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/storage"
	driftsync "github.com/drift/drift/internal/sync"
)

// setupSyncTest creates a temp remote root and redirects the global config
// to a temp path so tests don't touch the real ~/.drift/global.json.
// Returns the remote root path. Cleanup is registered automatically.
func setupSyncTest(t *testing.T) string {
	t.Helper()
	remoteRoot := t.TempDir()
	globalCfgPath := filepath.Join(t.TempDir(), "global.json")
	driftsync.SetGlobalConfigPathForTest(globalCfgPath)
	t.Cleanup(func() { driftsync.SetGlobalConfigPathForTest("") })
	return remoteRoot
}

// runSyncRemote runs `drift sync remote <path>` and returns its output.
func (h *TestHelper) runSyncRemote(path string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	syncShowRemote = false
	syncUnsetRemote = false
	return CaptureOutput(func() error {
		return syncRemoteCmd.RunE(syncRemoteCmd, []string{path})
	})
}

// runSyncEnable runs `drift sync enable`.
func (h *TestHelper) runSyncEnable() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return syncEnableCmd.RunE(syncEnableCmd, nil)
	})
}

// runSyncDisable runs `drift sync disable`.
func (h *TestHelper) runSyncDisable() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return syncDisableCmd.RunE(syncDisableCmd, nil)
	})
}

// runSyncStatus runs `drift sync status`.
func (h *TestHelper) runSyncStatus() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return syncStatusCmd.RunE(syncStatusCmd, nil)
	})
}

// runSyncNow runs `drift sync now`.
func (h *TestHelper) runSyncNow() (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return syncNowCmd.RunE(syncNowCmd, nil)
	})
}

// runClone runs `drift clone <project> [dest]`.
func (h *TestHelper) runClone(args ...string) (string, error) {
	h.T.Helper()
	h.SetupSharedState()
	return CaptureOutput(func() error {
		return cloneCmd.RunE(cloneCmd, args)
	})
}

// --- sync remote tests ---

func TestSyncRemote_Set(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)
	h.AssertContains(output, "Remote root set to")
	h.AssertContains(output, remoteRoot)

	// Verify it was persisted.
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if gcfg.RemoteRoot != remoteRoot {
		t.Errorf("expected RemoteRoot %q, got %q", remoteRoot, gcfg.RemoteRoot)
	}
}

func TestSyncRemote_Show(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set first.
	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)

	// Show.
	h.SetupSharedState()
	syncShowRemote = true
	syncUnsetRemote = false
	output, err := CaptureOutput(func() error {
		return syncRemoteCmd.RunE(syncRemoteCmd, nil)
	})
	h.AssertNoError(err)
	h.AssertContains(output, remoteRoot)
}

func TestSyncRemote_ShowEmpty(t *testing.T) {
	setupSyncTest(t) // creates empty global config
	h := NewTestHelper(t)
	h.InitProject()

	h.SetupSharedState()
	syncShowRemote = true
	syncUnsetRemote = false
	output, err := CaptureOutput(func() error {
		return syncRemoteCmd.RunE(syncRemoteCmd, nil)
	})
	h.AssertNoError(err)
	h.AssertContains(output, "No remote root configured")
}

func TestSyncRemote_Unset(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set first.
	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)

	// Unset.
	h.SetupSharedState()
	syncShowRemote = false
	syncUnsetRemote = true
	output, err := CaptureOutput(func() error {
		return syncRemoteCmd.RunE(syncRemoteCmd, nil)
	})
	h.AssertNoError(err)
	h.AssertContains(output, "Remote root unset")

	// Verify it was cleared.
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if gcfg.RemoteRoot != "" {
		t.Errorf("expected empty RemoteRoot, got %q", gcfg.RemoteRoot)
	}
}

func TestSyncRemote_InvalidPath(t *testing.T) {
	setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.runSyncRemote("/nonexistent/path/that/does/not/exist")
	h.AssertError(err)
}

// --- sync enable/disable tests ---

func TestSyncEnable_WithoutRemoteRoot(t *testing.T) {
	setupSyncTest(t) // empty global config
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.runSyncEnable()
	h.AssertError(err)
	if !strings.Contains(err.Error(), "no remote root configured") {
		t.Errorf("expected 'no remote root' error, got: %v", err)
	}
}

func TestSyncEnable_WithRemoteRoot(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set remote root first.
	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)

	// Enable sync.
	output, err := h.runSyncEnable()
	h.AssertNoError(err)
	h.AssertContains(output, "Sync enabled")

	// Verify config was updated.
	if !h.Config.Sync.Enabled {
		t.Error("expected Sync.Enabled to be true")
	}
	if h.Config.Sync.RemoteName == "" {
		t.Error("expected RemoteName to be set")
	}
	if h.Config.Sync.ProjectID == "" {
		t.Error("expected ProjectID to be set")
	}

	// Verify remote directory was created.
	remoteDir := filepath.Join(remoteRoot, h.Config.Sync.RemoteName)
	if _, err := os.Stat(remoteDir); err != nil {
		t.Errorf("expected remote dir %s to exist: %v", remoteDir, err)
	}
}

func TestSyncDisable(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set up and enable.
	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)
	_, err = h.runSyncEnable()
	h.AssertNoError(err)

	// Disable.
	output, err := h.runSyncDisable()
	h.AssertNoError(err)
	h.AssertContains(output, "Sync disabled")

	if h.Config.Sync.Enabled {
		t.Error("expected Sync.Enabled to be false after disable")
	}
}

func TestSyncDisable_NotEnabled(t *testing.T) {
	setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.runSyncDisable()
	h.AssertNoError(err)
	h.AssertContains(output, "not enabled")
}

// --- sync status tests ---

func TestSyncStatus_NotEnabled(t *testing.T) {
	setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	output, err := h.runSyncStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "not enabled")
}

func TestSyncStatus_Enabled(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)
	_, err = h.runSyncEnable()
	h.AssertNoError(err)

	output, err := h.runSyncStatus()
	h.AssertNoError(err)
	h.AssertContains(output, "Enabled:  yes")
	h.AssertContains(output, "never")
}

// --- sync now tests ---

func TestSyncNow_NotEnabled(t *testing.T) {
	setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.runSyncNow()
	h.AssertError(err)
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("expected 'not enabled' error, got: %v", err)
	}
}

func TestSyncNow_PushPullFiles(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set up sync.
	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)
	_, err = h.runSyncEnable()
	h.AssertNoError(err)

	// Create a local file and sync (push).
	h.WriteFile("chapter1.txt", "hello world")
	output, err := h.runSyncNow()
	h.AssertNoError(err)
	h.AssertContains(output, "Sync complete")

	// Verify file was pushed to remote.
	remoteFile := filepath.Join(remoteRoot, h.Config.Sync.RemoteName, "chapter1.txt")
	data, err := os.ReadFile(remoteFile)
	if err != nil {
		t.Fatalf("expected remote file to exist: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}

	// Create a file on the remote side and sync (pull).
	remoteOnly := filepath.Join(remoteRoot, h.Config.Sync.RemoteName, "remote-only.txt")
	if err := os.WriteFile(remoteOnly, []byte("from remote"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = h.runSyncNow()
	h.AssertNoError(err)

	// Verify file was pulled to local.
	localData, err := os.ReadFile(filepath.Join(h.Dir, "remote-only.txt"))
	if err != nil {
		t.Fatalf("expected local file to exist after pull: %v", err)
	}
	if string(localData) != "from remote" {
		t.Errorf("expected 'from remote', got %q", string(localData))
	}
}

// --- clone tests ---

func TestClone_Basic(t *testing.T) {
	remoteRoot := setupSyncTest(t)

	// Create a source project, add files, and sync to remote.
	src := NewTestHelper(t)
	src.InitProject()
	src.WriteFile("novel.txt", "chapter 1 content")

	_, err := src.runSyncRemote(remoteRoot)
	src.AssertNoError(err)
	_, err = src.runSyncEnable()
	src.AssertNoError(err)
	_, err = src.runSyncNow()
	src.AssertNoError(err)

	// Now clone from remote into a new directory.
	cloneDest := filepath.Join(t.TempDir(), "cloned-novel")
	output, err := src.runClone(src.Config.Sync.RemoteName, cloneDest)
	src.AssertNoError(err)
	src.AssertContains(output, "Cloned")

	// Verify the cloned project has the file.
	clonedFile := filepath.Join(cloneDest, "novel.txt")
	data, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatalf("expected cloned file to exist: %v", err)
	}
	if string(data) != "chapter 1 content" {
		t.Errorf("expected 'chapter 1 content', got %q", string(data))
	}

	// Verify .drift/ was cloned too.
	if _, err := os.Stat(filepath.Join(cloneDest, ".drift")); err != nil {
		t.Errorf("expected .drift/ to exist in clone: %v", err)
	}
}

func TestClone_ProjectNotFound(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)

	_, err = h.runClone("nonexistent-project", t.TempDir())
	h.AssertError(err)
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestClone_NoRemoteRoot(t *testing.T) {
	setupSyncTest(t) // empty global config
	h := NewTestHelper(t)
	h.InitProject()

	_, err := h.runClone("some-project")
	h.AssertError(err)
	if !strings.Contains(err.Error(), "no remote root") {
		t.Errorf("expected 'no remote root' error, got: %v", err)
	}
}

func TestClone_DestNotEmpty(t *testing.T) {
	remoteRoot := setupSyncTest(t)

	// Set up source project and sync.
	src := NewTestHelper(t)
	src.InitProject()
	src.WriteFile("a.txt", "a")
	_, err := src.runSyncRemote(remoteRoot)
	src.AssertNoError(err)
	_, err = src.runSyncEnable()
	src.AssertNoError(err)
	_, err = src.runSyncNow()
	src.AssertNoError(err)

	// Create a non-empty destination.
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "existing.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = src.runClone(src.Config.Sync.RemoteName, dest)
	src.AssertError(err)
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("expected 'not empty' error, got: %v", err)
	}
}

// --- project ID tests ---

func TestNewProjectID_IsUniqueHex(t *testing.T) {
	id1 := driftsync.NewProjectID()
	id2 := driftsync.NewProjectID()

	if id1 == "" || id2 == "" {
		t.Error("expected non-empty project IDs")
	}
	if id1 == id2 {
		t.Error("expected unique project IDs")
	}
	if len(id1) != 32 { // 16 bytes hex = 32 chars
		t.Errorf("expected 32-char hex ID, got %d chars: %q", len(id1), id1)
	}
}

// --- transport tests (unit tests for the sync package via CLI) ---

func TestLocalTransport_ProjectExists(t *testing.T) {
	remoteRoot := t.TempDir()
	transport := driftsync.NewLocalTransport(remoteRoot)

	// No projects initially.
	exists, err := transport.ProjectExists("myproject")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected project to not exist")
	}

	// Create a project directory with .drift/.
	projectDir := filepath.Join(remoteRoot, "myproject")
	if err := os.MkdirAll(filepath.Join(projectDir, ".drift"), 0755); err != nil {
		t.Fatal(err)
	}

	exists, err = transport.ProjectExists("myproject")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected project to exist")
	}
}

func TestLocalTransport_ListProjects(t *testing.T) {
	remoteRoot := t.TempDir()
	transport := driftsync.NewLocalTransport(remoteRoot)

	// Create two projects (with .drift/) and one non-project dir.
	for _, name := range []string{"project-a", "project-b"} {
		dir := filepath.Join(remoteRoot, name, ".drift")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Non-project dir (no .drift/).
	if err := os.MkdirAll(filepath.Join(remoteRoot, "not-a-project"), 0755); err != nil {
		t.Fatal(err)
	}

	projects, err := transport.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d: %v", len(projects), projects)
	}
}

// loadConfigFromStore is a helper to load config from a store.
func loadConfigFromStore(store *storage.Store) (*config.Config, error) {
	return config.LoadConfig(store.DriftDir())
}
