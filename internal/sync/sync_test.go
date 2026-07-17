package sync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/remote"
)

// saveTestCredential writes a credential for the given remote name to the
// user-level credentials.json, restoring the original in cleanup. This is
// necessary because credentials are stored at the OS user-config level,
// not per-project, and PushToRemote/PullFromRemote resolve them via
// remote.LoadCredentials before acquiring the workspace lock.
func saveTestCredential(t *testing.T, remoteName, password string) {
	t.Helper()
	cred, err := remote.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	origData, err := json.Marshal(cred)
	if err != nil {
		t.Fatalf("marshal original credentials: %v", err)
	}
	cred.AddOrUpdateCredential(remote.Credential{
		Remote:   remoteName,
		Host:     "127.0.0.1",
		User:     "testuser",
		Password: password,
	})
	if err := remote.SaveCredentials(cred); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	t.Cleanup(func() {
		var origCred remote.CredentialsFile
		if err := json.Unmarshal(origData, &origCred); err == nil {
			remote.SaveCredentials(&origCred)
		}
	})
}

// writeTestRemote writes a remote entry to .drift/remotes.json in workDir
// so resolveRemoteConfig can find it.
func writeTestRemote(t *testing.T, workDir, remoteName, url string) {
	t.Helper()
	driftDir := workDir + "/.drift"
	rf, err := remote.LoadRemotes(driftDir)
	if err != nil {
		t.Fatalf("LoadRemotes: %v", err)
	}
	rf.AddOrUpdateRemote(remote.RemoteConfig{
		Name: remoteName,
		Type: "webdav",
		URL:  url,
		User: "testuser",
	})
	if err := remote.SaveRemotes(driftDir, rf); err != nil {
		t.Fatalf("SaveRemotes: %v", err)
	}
}

// TestPushToRemote_NotADriftRepo verifies that calling PushToRemote on a
// directory without .drift/ fails with "not a drift repository" before
// reaching the workspace lock or remote resolution.
func TestPushToRemote_NotADriftRepo(t *testing.T) {
	store := setupTestStore(t)
	dir := t.TempDir() // no .drift/ directory

	_, err := PushToRemote(context.Background(), store, dir, "origin", "", false)
	if err == nil {
		t.Fatal("expected error for non-repo dir, got nil")
	}
	if !strings.Contains(err.Error(), "not a drift repository") {
		t.Errorf("expected 'not a drift repository', got: %v", err)
	}
}

// TestPushToRemote_RemoteNotFound verifies that calling PushToRemote with a
// remote name not in remotes.json fails with "not found" (wrapping
// os.ErrNotExist).
func TestPushToRemote_RemoteNotFound(t *testing.T) {
	store, dir := setupLockedProject(t)

	_, err := PushToRemote(context.Background(), store, dir, "nonexistent-remote", "", false)
	if err == nil {
		t.Fatal("expected error for unconfigured remote, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got: %v", err)
	}
}

// TestPushToRemote_NoCredential verifies that a configured remote without a
// matching credential fails with "no credential" before the workspace lock
// is acquired.
func TestPushToRemote_NoCredential(t *testing.T) {
	// Insecure http is refused by default; opt in so the test reaches the
	// credential-resolution step rather than failing on scheme.
	t.Setenv("DRIFT_ALLOW_INSECURE", "1")

	store, dir := setupLockedProject(t)
	writeTestRemote(t, dir, "test-nocred-remote", "http://127.0.0.1:1/repo")

	_, err := PushToRemote(context.Background(), store, dir, "test-nocred-remote", "", false)
	if err == nil {
		t.Fatal("expected error for missing credential, got nil")
	}
	if !strings.Contains(err.Error(), "no credential") {
		t.Errorf("expected error containing 'no credential', got: %v", err)
	}
}

// TestPushToRemote_WorkspaceLockHeld verifies that PushToRemote fails with
// ErrLocked when the workspace lock is already held by the same process.
// This requires the remote config and credentials to be fully resolved so
// the function reaches the lock-acquisition step.
func TestPushToRemote_WorkspaceLockHeld(t *testing.T) {
	// Insecure http is refused by default (see resolveRemoteConfigWithWarn).
	// This test exercises the lock-acquisition path, not the scheme check,
	// so opt in for the duration of the test.
	t.Setenv("DRIFT_ALLOW_INSECURE", "1")

	store, dir := setupLockedProject(t)
	writeTestRemote(t, dir, "origin", "http://127.0.0.1:1/repo")
	saveTestCredential(t, "origin", "testpass")

	// Acquire the lock so PushToRemote's acquisition fails.
	if err := AcquireWorkspaceLock(dir); err != nil {
		t.Fatalf("AcquireWorkspaceLock: %v", err)
	}
	defer ReleaseWorkspaceLock(dir)

	_, err := PushToRemote(context.Background(), store, dir, "origin", "", false)
	if err == nil {
		t.Fatal("expected ErrLocked when lock is held, got nil")
	}
	if !errors.Is(err, ErrLocked) {
		t.Errorf("expected error wrapping ErrLocked, got: %v", err)
	}
}

// TestPullFromRemote_RemoteNotFound verifies the same error path for Pull.
func TestPullFromRemote_RemoteNotFound(t *testing.T) {
	store, dir := setupLockedProject(t)

	_, err := PullFromRemote(context.Background(), store, dir, "nonexistent-remote", "", false)
	if err == nil {
		t.Fatal("expected error for unconfigured remote, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got: %v", err)
	}
}

// TestPullFromRemote_WorkspaceLockHeld verifies that PullFromRemote also
// fails with ErrLocked when the workspace lock is held.
func TestPullFromRemote_WorkspaceLockHeld(t *testing.T) {
	// See TestPushToRemote_WorkspaceLockHeld: opt into insecure http so the
	// test reaches the lock-acquisition step rather than failing on scheme.
	t.Setenv("DRIFT_ALLOW_INSECURE", "1")

	store, dir := setupLockedProject(t)
	writeTestRemote(t, dir, "origin", "http://127.0.0.1:1/repo")
	saveTestCredential(t, "origin", "testpass")

	if err := AcquireWorkspaceLock(dir); err != nil {
		t.Fatalf("AcquireWorkspaceLock: %v", err)
	}
	defer ReleaseWorkspaceLock(dir)

	_, err := PullFromRemote(context.Background(), store, dir, "origin", "", false)
	if err == nil {
		t.Fatal("expected ErrLocked when lock is held, got nil")
	}
	if !errors.Is(err, ErrLocked) {
		t.Errorf("expected error wrapping ErrLocked, got: %v", err)
	}
}
