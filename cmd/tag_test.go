package cmd

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestTag_List(t *testing.T) {
	setupTestRepo(t)

	out := captureStdout(t, func() {
		if err := runCmd(tagListCmd, nil); err != nil {
			t.Fatalf("tag list: %v", err)
		}
	})

	if !strings.Contains(out, "Tags") {
		t.Errorf("stdout = %q, want 'Tags'", out)
	}
}

func TestTag_AddAndDelete(t *testing.T) {
	workDir := setupTestRepo(t)
	sid := saveSnapshot(t, workDir, "f.txt", "data", "initial")

	// Add a tag.
	addOut := captureStdout(t, func() {
		if err := runCmd(tagAddCmd, []string{"v1.0", "id:" + sid}); err != nil {
			t.Fatalf("tag add: %v", err)
		}
	})
	if !strings.Contains(addOut, "Tag added") {
		t.Errorf("stdout = %q, want 'Tag added'", addOut)
	}

	// Verify the tag shows up in the list.
	listOut := captureStdout(t, func() {
		if err := runCmd(tagListCmd, nil); err != nil {
			t.Fatalf("tag list: %v", err)
		}
	})
	if !strings.Contains(listOut, "v1.0") {
		t.Errorf("tag list = %q, want 'v1.0'", listOut)
	}

	// Delete the tag.
	delOut := captureStdout(t, func() {
		if err := runCmd(tagDeleteCmd, []string{"v1.0"}); err != nil {
			t.Fatalf("tag delete: %v", err)
		}
	})
	if !strings.Contains(delOut, "Tag deleted") {
		t.Errorf("stdout = %q, want 'Tag deleted'", delOut)
	}

	// Verify the tag is gone.
	listOut2 := captureStdout(t, func() {
		if err := runCmd(tagListCmd, nil); err != nil {
			t.Fatalf("tag list: %v", err)
		}
	})
	if strings.Contains(listOut2, "v1.0") {
		t.Errorf("tag list = %q, want 'v1.0' to be gone", listOut2)
	}
}

func TestTag_AddDefaultHead(t *testing.T) {
	workDir := setupTestRepo(t)
	saveSnapshot(t, workDir, "f.txt", "data", "initial")

	// Add a tag without specifying a version — should default to HEAD.
	addOut := captureStdout(t, func() {
		if err := runCmd(tagAddCmd, []string{"v2.0"}); err != nil {
			t.Fatalf("tag add (default head): %v", err)
		}
	})
	if !strings.Contains(addOut, "Tag added") {
		t.Errorf("stdout = %q, want 'Tag added'", addOut)
	}

	// Verify the tag appears in the list.
	listOut := captureStdout(t, func() {
		if err := runCmd(tagListCmd, nil); err != nil {
			t.Fatalf("tag list: %v", err)
		}
	})
	if !strings.Contains(listOut, "v2.0") {
		t.Errorf("tag list = %q, want 'v2.0'", listOut)
	}
}

func TestTag_AddInvalidSnapshot(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(tagAddCmd, []string{"v1.0", "id:nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestTag_DeleteNonexistent(t *testing.T) {
	setupTestRepo(t)

	errOut := captureStderr(t, func() {
		if err := runCmd(tagDeleteCmd, []string{"nonexistent"}); !errors.Is(err, ErrSilent) {
			t.Errorf("expected ErrSilent, got %v", err)
		}
	})

	if !strings.Contains(errOut, "not found") {
		t.Errorf("stderr = %q, want 'not found'", strings.TrimSpace(errOut))
	}
}

func TestTag_JSON(t *testing.T) {
	setupTestRepo(t)
	globalJSON = true

	out := captureStdout(t, func() {
		if err := runCmd(tagListCmd, nil); err != nil {
			t.Fatalf("tag list: %v", err)
		}
	})

	var env JSONEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if env.Command != "tag" || env.Status != "ok" {
		t.Errorf("envelope = %+v, want command=tag status=ok", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %T", env.Data)
	}
	tags, _ := data["tags"].([]interface{})
	if len(tags) != 0 {
		t.Errorf("data.tags has %d entries, want 0", len(tags))
	}
}
