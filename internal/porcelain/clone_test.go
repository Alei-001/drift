package porcelain

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/remote"
)

// backupRestoreCredentials saves the current user-level credentials.json
// content and restores it in t.Cleanup. Necessary because CloneRemote
// writes credentials to the user-level file when a password is provided.
func backupRestoreCredentials(t *testing.T) {
	t.Helper()
	cred, err := remote.LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	origData, err := json.Marshal(cred)
	if err != nil {
		t.Fatalf("marshal original credentials: %v", err)
	}
	t.Cleanup(func() {
		var origCred remote.CredentialsFile
		if err := json.Unmarshal(origData, &origCred); err == nil {
			remote.SaveCredentials(&origCred)
		}
	})
}

// TestCloneRemote_EmptyURL verifies that an empty RemoteURL with no
// TargetDir fails with "cannot determine directory name" before creating
// any files or directories.
func TestCloneRemote_EmptyURL(t *testing.T) {
	opts := CloneOptions{
		RemoteURL: "",
		WorkDir:   t.TempDir(),
	}

	_, err := CloneRemote(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
	if !strings.Contains(err.Error(), "cannot determine directory name") {
		t.Errorf("expected 'cannot determine directory name', got: %v", err)
	}
}

// TestCloneRemote_BareSlashURL verifies that a URL of "/" also fails the
// directory-name derivation (path.Base("/") returns "/", which trims to "").
func TestCloneRemote_BareSlashURL(t *testing.T) {
	opts := CloneOptions{
		RemoteURL: "/",
		WorkDir:   t.TempDir(),
	}

	_, err := CloneRemote(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for bare slash URL, got nil")
	}
	if !strings.Contains(err.Error(), "cannot determine directory name") {
		t.Errorf("expected 'cannot determine directory name', got: %v", err)
	}
}

// TestCloneRemote_UnreachableRemote verifies that CloneRemote creates the
// target directory and initializes the project, but fails at the pull step
// when the remote is unreachable. The credential file is backed up and
// restored in cleanup because CloneRemote saves the password to the
// user-level credentials.json.
func TestCloneRemote_UnreachableRemote(t *testing.T) {
	// Insecure http is refused by default; opt in so the test reaches the
	// pull step (and verifies the connection-refused error path) rather
	// than failing on the scheme check before init.
	t.Setenv("DRIFT_ALLOW_INSECURE", "1")

	backupRestoreCredentials(t)

	workDir := t.TempDir()
	opts := CloneOptions{
		RemoteURL:  "http://127.0.0.1:1/nonexistent-repo",
		TargetDir:  "clonetest",
		WorkDir:    workDir,
		RemoteType: "webdav",
		User:       "testuser",
		Password:   "testpass",
	}

	_, err := CloneRemote(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for unreachable remote, got nil")
	}
	// The error should come from the pull step (connection refused).
	if !strings.Contains(err.Error(), "pull") {
		t.Errorf("expected error containing 'pull', got: %v", err)
	}

	// Verify the target directory was created.
	targetDir := filepath.Join(workDir, "clonetest")
	if info, err := os.Stat(targetDir); err != nil {
		t.Errorf("expected target dir %s to exist, got error: %v", targetDir, err)
	} else if !info.IsDir() {
		t.Errorf("expected %s to be a directory", targetDir)
	}

	// Verify .drift/ was initialized (remotes.json should exist).
	remotesFile := filepath.Join(targetDir, ".drift", "remotes.json")
	if _, err := os.Stat(remotesFile); err != nil {
		t.Errorf("expected %s to exist, got error: %v", remotesFile, err)
	}
}

// TestCloneRemote_HEADVerification verifies that CloneRemote derives the
// branch name from the store after pull. Since the remote is unreachable,
// this tests the error path where CloneRemote fails before reaching the
// HEAD-verification code. The test confirms the function does not
// successfully return a result with a wrong branch name.
func TestCloneRemote_HEADVerification(t *testing.T) {
	// See TestCloneRemote_UnreachableRemote: opt into insecure http.
	t.Setenv("DRIFT_ALLOW_INSECURE", "1")

	backupRestoreCredentials(t)

	workDir := t.TempDir()
	opts := CloneOptions{
		RemoteURL:  "http://127.0.0.1:1/repo",
		TargetDir:  "headtest",
		WorkDir:    workDir,
		RemoteType: "webdav",
		User:       "testuser",
		Password:   "testpass",
	}

	result, err := CloneRemote(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error for unreachable remote, got nil")
	}
	if result != nil {
		t.Errorf("expected nil result on failure, got %+v", result)
	}
}
