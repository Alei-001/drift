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
	h.AssertContains(output, "Remote set:")
	h.AssertContains(output, remoteRoot)

	// Verify it was persisted.
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if gcfg.Path != remoteRoot {
		t.Errorf("expected Path %q, got %q", remoteRoot, gcfg.Path)
	}
	if gcfg.Protocol != "local" {
		t.Errorf("expected Protocol %q, got %q", "local", gcfg.Protocol)
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
	h.AssertContains(output, "local")
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
	h.AssertContains(output, "No remote configured")
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
	h.AssertContains(output, "Remote unset")

	// Verify it was cleared.
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if gcfg.Protocol != "" {
		t.Errorf("expected empty Protocol, got %q", gcfg.Protocol)
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
	if !strings.Contains(err.Error(), "no remote configured") {
		t.Errorf("expected 'no remote configured' error, got: %v", err)
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
	if !strings.Contains(err.Error(), "no remote configured") {
		t.Errorf("expected 'no remote configured' error, got: %v", err)
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

// --- auto-sync tests ---

// TestAutoSyncAfterSave_NoopWhenDisabled verifies that auto-sync does
// nothing when sync is not enabled.
func TestAutoSyncAfterSave_NoopWhenDisabled(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set remote but don't enable sync.
	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)

	// Auto-sync should be a no-op.
	h.SetupSharedState()
	AutoSyncAfterSave(h.Dir, h.Config, h.Store)

	// Verify nothing was synced to remote.
	entries, err := os.ReadDir(remoteRoot)
	if err != nil {
		t.Fatal(err)
	}
	// remoteRoot should only contain the project directory (created by
	// sync remote, not by sync enable). Actually sync remote doesn't
	// create project dirs, so remoteRoot should be empty.
	for _, e := range entries {
		// The project dir is only created by sync enable, which we didn't call.
		t.Errorf("unexpected entry in remote: %s", e.Name())
	}
}

// TestAutoSyncAfterSave_SyncsOnSave verifies that auto-sync pushes files
// after a save when sync is enabled.
func TestAutoSyncAfterSave_SyncsOnSave(t *testing.T) {
	remoteRoot := setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set up sync.
	_, err := h.runSyncRemote(remoteRoot)
	h.AssertNoError(err)
	_, err = h.runSyncEnable()
	h.AssertNoError(err)

	// Create a file and add it.
	h.WriteFile("chapter1.txt", "hello world")
	h.RunAdd("chapter1.txt")

	// Save — this should trigger auto-sync.
	h.RunSave("test save")

	// Verify the file was synced to remote.
	remoteFile := filepath.Join(remoteRoot, h.Config.Sync.RemoteName, "chapter1.txt")
	data, err := os.ReadFile(remoteFile)
	if err != nil {
		t.Fatalf("expected file to be synced to remote: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}

	// Verify last sync timestamp was updated.
	if h.Config.Sync.LastSync == "" {
		t.Error("expected LastSync to be set after auto-sync")
	}
}

// TestAutoSyncAfterSave_NoRemoteConfigured verifies that auto-sync is
// silent when no remote is configured.
func TestAutoSyncAfterSave_NoRemoteConfigured(t *testing.T) {
	setupSyncTest(t) // empty global config
	h := NewTestHelper(t)
	h.InitProject()

	// Enable sync (will fail because no remote).
	_, err := h.runSyncEnable()
	h.AssertError(err)

	// Even if we force enabled=true, auto-sync should be silent.
	h.Config.Sync.Enabled = true
	h.SetupSharedState()
	AutoSyncAfterSave(h.Dir, h.Config, h.Store)
	// No error, no panic — that's the test.
}

// --- WebDAV config tests ---

// TestSyncRemote_WebDAV verifies that a WebDAV URL is stored correctly.
func TestSyncRemote_WebDAV(t *testing.T) {
	setupSyncTest(t)
	h := NewTestHelper(t)
	h.InitProject()

	// Set WebDAV remote with credentials via flags.
	h.SetupSharedState()
	syncShowRemote = false
	syncUnsetRemote = false
	syncUser = "alice"
	syncPass = "secret"
	output, err := CaptureOutput(func() error {
		return syncRemoteCmd.RunE(syncRemoteCmd, []string{"https://cloud.example.com/dav"})
	})
	h.AssertNoError(err)
	h.AssertContains(output, "Remote set:")

	// Verify it was persisted.
	gcfg, err := driftsync.LoadGlobalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if gcfg.Protocol != "webdav" {
		t.Errorf("expected Protocol %q, got %q", "webdav", gcfg.Protocol)
	}
	if gcfg.Host != "cloud.example.com" {
		t.Errorf("expected Host %q, got %q", "cloud.example.com", gcfg.Host)
	}
	if gcfg.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", gcfg.Username)
	}
	if gcfg.Password != "secret" {
		t.Errorf("expected password 'secret', got %q", gcfg.Password)
	}
	if !gcfg.TLS {
		t.Error("expected TLS to be true for https URL")
	}
}

// TestGetRemoteType verifies remote type detection.
func TestGetRemoteType(t *testing.T) {
	// None.
	gcfg := &driftsync.GlobalConfig{}
	if gcfg.GetRemoteType() != driftsync.RemoteNone {
		t.Error("expected RemoteNone")
	}

	// Local.
	gcfg.Protocol = "local"
	if gcfg.GetRemoteType() != driftsync.RemoteLocal {
		t.Error("expected RemoteLocal")
	}

	// WebDAV.
	gcfg.Protocol = "webdav"
	if gcfg.GetRemoteType() != driftsync.RemoteWebDAV {
		t.Error("expected RemoteWebDAV")
	}

	// FTP.
	gcfg.Protocol = "ftp"
	if gcfg.GetRemoteType() != driftsync.RemoteFTP {
		t.Error("expected RemoteFTP")
	}

	// SFTP.
	gcfg.Protocol = "sftp"
	if gcfg.GetRemoteType() != driftsync.RemoteSFTP {
		t.Error("expected RemoteSFTP")
	}

	// SMB.
	gcfg.Protocol = "smb"
	if gcfg.GetRemoteType() != driftsync.RemoteSMB {
		t.Error("expected RemoteSMB")
	}
}
