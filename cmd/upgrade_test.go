package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/version"
)

// fakeUpgradeServer is a minimal GitHub releases API stub for testing
// cmd/upgrade.go. It responds to /repos/.../releases/latest with a configurable
// status code, letting us exercise the error-output paths of runUpgrade.
type fakeUpgradeServer struct {
	t          *testing.T
	server     *httptest.Server
	statusCode int
	body       string // JSON body for the releases/latest response
}

func newFakeUpgradeServer(t *testing.T, status int, body string) *fakeUpgradeServer {
	f := &fakeUpgradeServer{t: t, statusCode: status, body: body}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeUpgradeServer) close()      { f.server.Close() }
func (f *fakeUpgradeServer) url() string { return f.server.URL }

func (f *fakeUpgradeServer) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.statusCode)
	w.Write([]byte(f.body))
}

// withGlobals sets globalJSON/globalQuiet for the duration of a test and
// restores them afterwards.
func withGlobals(jsonMode, quietMode bool, fn func()) {
	oldJSON, oldQuiet := globalJSON, globalQuiet
	globalJSON = jsonMode
	globalQuiet = quietMode
	defer func() { globalJSON, globalQuiet = oldJSON, oldQuiet }()
	fn()
}

func TestRunUpgrade_Check_NoRelease_Human(t *testing.T) {
	srv := newFakeUpgradeServer(t, http.StatusNotFound, `{"message":"Not Found"}`)
	defer srv.close()

	oldURL := upgradeAPIURL
	upgradeAPIURL = srv.url()
	defer func() { upgradeAPIURL = oldURL }()

	upgradeCheck = true
	defer func() { upgradeCheck = false }()

	// Capture stderr (statusFailed writes there).
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	withGlobals(false, false, func() {
		err := runUpgrade(nil, nil)
		if !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})
	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, ">>> Upgrade [failed]") {
		t.Errorf("stderr = %q, want '>>> Upgrade [failed]'", output)
	}
	if !strings.Contains(output, "no GitHub release has been published") {
		t.Errorf("stderr = %q, want hint about no release", output)
	}
}

func TestRunUpgrade_Check_NoRelease_JSON(t *testing.T) {
	srv := newFakeUpgradeServer(t, http.StatusNotFound, `{"message":"Not Found"}`)
	defer srv.close()

	oldURL := upgradeAPIURL
	upgradeAPIURL = srv.url()
	defer func() { upgradeAPIURL = oldURL }()

	upgradeCheck = true
	defer func() { upgradeCheck = false }()

	// JSON mode writes the failure envelope to stdout via outputJSON.
	out := captureStdout(t, func() {
		withGlobals(true, false, func() {
			err := runUpgrade(nil, nil)
			if !errors.Is(err, ErrSilent) {
				t.Errorf("expected ErrSilent, got %v", err)
			}
		})
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "upgrade" || env.Status != "failed" {
		t.Errorf("envelope = %+v, want command=upgrade status=failed", env)
	}
	if env.Hint == nil || !strings.Contains(*env.Hint, "no GitHub release") {
		t.Errorf("hint = %v, want mention of 'no GitHub release'", env.Hint)
	}
}

func TestRunUpgrade_Check_UpToDate(t *testing.T) {
	// Simulate a release at the same version as the running binary.
	srv := newFakeUpgradeServer(t, http.StatusOK, `{"tag_name":"v9.9.9","assets":[]}`)
	defer srv.close()

	oldURL := upgradeAPIURL
	upgradeAPIURL = srv.url()
	defer func() { upgradeAPIURL = oldURL }()

	oldVer := version.Version
	version.Version = "v9.9.9"
	defer func() { version.Version = oldVer }()

	upgradeCheck = true
	defer func() { upgradeCheck = false }()

	out := captureStdout(t, func() {
		withGlobals(false, false, func() {
			err := runUpgrade(nil, nil)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	})

	if !strings.Contains(out, ">>> Upgrade [ok]") {
		t.Errorf("stdout = %q, want '>>> Upgrade [ok]'", out)
	}
	if !strings.Contains(out, "already up to date") {
		t.Errorf("stdout = %q, want 'already up to date'", out)
	}
}

// Ensure that context cancellation is respected by the upgrade flow (the HTTP
// client should not hang when the context is already cancelled).
func TestRunUpgrade_CancelledContext(t *testing.T) {
	srv := newFakeUpgradeServer(t, http.StatusOK, `{"tag_name":"v9.9.9","assets":[]}`)
	defer srv.close()

	oldURL := upgradeAPIURL
	upgradeAPIURL = srv.url()
	defer func() { upgradeAPIURL = oldURL }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := version.Upgrade(ctx, "v0.1.0", version.UpgradeOptions{
		Check:  true,
		APIURL: srv.url(),
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}
