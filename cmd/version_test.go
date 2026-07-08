package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/version"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns whatever was
// written. The original stdout is restored even on panic.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestRunVersion_Human(t *testing.T) {
	// Inject a known version so the output is deterministic.
	old := version.Version
	version.Version = "v9.9.9"
	defer func() { version.Version = old }()

	globalJSON = false
	globalQuiet = false

	out := captureStdout(t, func() {
		if err := runVersion(nil, nil); err != nil {
			t.Fatal(err)
		}
	})

	// Line 1: "drift v9.9.9 (commit: ..., built: ...)"
	// Line 2: "  <goversion>  <os>/<arch>"
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected >=2 lines, got %d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "drift v9.9.9 ") {
		t.Errorf("line 1 = %q, want prefix %q", lines[0], "drift v9.9.9 ")
	}
	wantPlatform := runtime.GOOS + "/" + runtime.GOARCH
	if !strings.HasSuffix(lines[1], wantPlatform) {
		t.Errorf("line 2 = %q, want suffix %q", lines[1], wantPlatform)
	}
}

func TestRunVersion_JSON(t *testing.T) {
	old := version.Version
	version.Version = "v9.9.9"
	defer func() { version.Version = old }()

	globalJSON = true
	globalQuiet = false
	defer func() { globalJSON = false }()

	out := captureStdout(t, func() {
		if err := runVersion(nil, nil); err != nil {
			t.Fatal(err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "version" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=version status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	if data["version"] != "v9.9.9" {
		t.Errorf("data.version = %v, want v9.9.9", data["version"])
	}
	if data["os"] != runtime.GOOS || data["arch"] != runtime.GOARCH {
		t.Errorf("platform = %v/%v, want %s/%s", data["os"], data["arch"], runtime.GOOS, runtime.GOARCH)
	}
}

func TestRunVersion_Quiet(t *testing.T) {
	old := version.Version
	version.Version = "v9.9.9"
	defer func() { version.Version = old }()

	globalJSON = false
	globalQuiet = true
	defer func() { globalQuiet = false }()

	out := captureStdout(t, func() {
		if err := runVersion(nil, nil); err != nil {
			t.Fatal(err)
		}
	})

	// Quiet mode prints just the bare version string.
	got := strings.TrimSpace(out)
	if got != "v9.9.9" {
		t.Errorf("quiet output = %q, want %q", got, "v9.9.9")
	}
}
