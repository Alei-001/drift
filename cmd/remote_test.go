package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRemote_List(t *testing.T) {
	setupTestRepo(t)

	out := captureStdout(t, func() {
		if err := runCmd(remoteListCmd, nil); err != nil {
			t.Fatalf("remote list: %v", err)
		}
	})

	if !strings.Contains(out, "no remotes configured") {
		t.Errorf("stdout = %q, want 'no remotes configured'", out)
	}
}

func TestRemote_RemoveNonexistent(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(remoteRemoveCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestRemote_ShowNonexistent(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(remoteShowCmd, []string{"origin"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestRemote_List_JSON(t *testing.T) {
	setupTestRepo(t)
	globalJSON = true

	out := captureStdout(t, func() {
		if err := runCmd(remoteListCmd, nil); err != nil {
			t.Fatalf("remote list: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "remote list" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=remote list status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	remotes, _ := data["remotes"].([]interface{})
	if len(remotes) != 0 {
		t.Errorf("data.remotes has %d entries, want 0", len(remotes))
	}
}

func TestRemote_NotARepo(t *testing.T) {
	setupEmptyDir(t)

	// remote list uses loadRemotesOrReport which checks for .drift/ and
	// returns a wrapped "not a drift repository" error. Unlike commands
	// that go through openProjectOrReport, remoteListCmd returns the raw
	// error (not ErrSilent), so we just check it's non-nil and mentions
	// "not a drift repository".
	err := runCmd(remoteListCmd, nil)
	if err == nil {
		t.Fatal("expected error from remote list without a repo, got nil")
	}
	if !strings.Contains(err.Error(), "not a drift repository") {
		t.Errorf("error = %q, want 'not a drift repository'", err.Error())
	}
}
