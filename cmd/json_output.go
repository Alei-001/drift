package cmd

import (
	"encoding/json"
	"fmt"
	"os"
)

// JSONEnvelope is the unified output envelope for --json mode.
// All commands supporting --json emit this structure so that scripts can
// parse a single schema regardless of the command. Hint is a pointer so
// that an absent hint serializes as JSON null (per docs/cli-design.md),
// matching the design's "hint": null example, rather than being omitted.
// ErrorDetail is populated only when --verbose is set and an underlying
// error is available, so scripts can access the real error cause.
type JSONEnvelope struct {
	Command     string      `json:"command"`
	Status      string      `json:"status"`                 // ok / failed / warning / active
	Error       string      `json:"error,omitempty"`        // error message when status == "failed"
	ErrorDetail string      `json:"error_detail,omitempty"` // verbose: underlying error cause
	Data        interface{} `json:"data,omitempty"`
	Hint        *string     `json:"hint"`
}

// outputJSON prints the envelope as JSON to stdout. When globalQuiet is set
// and the status is "ok", no output is produced (quiet takes precedence over
// JSON for successful operations). Warnings and failures are always printed
// so that partial failures remain visible. HTML escaping is disabled so
// that user content (messages, tag names, paths) is emitted verbatim.
func outputJSON(envelope JSONEnvelope) error {
	if globalQuiet && envelope.Status == "ok" {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(envelope)
}

// hintPtr returns a pointer to s when s is non-empty, or nil otherwise.
// Use it to populate JSONEnvelope.Hint so that an absent hint serializes
// as JSON null rather than an empty string.
func hintPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// reportFailed reports a command failure via JSON (when globalJSON is set)
// or the human-friendly statusFailed block. The caller should return
// ErrSilent (or silentWrap(err)) after calling this so that Execute exits
// with code 1 without re-printing the error. action is the display label
// (e.g. "Save"); command is the JSON "command" field value (e.g. "save").
//
// err is the underlying error that caused the failure. When --verbose is
// set, its message is appended to the human-readable output as a "detail"
// line and included in the JSON envelope's error_detail field. Pass nil
// when no underlying error is available (e.g. parameter validation).
func reportFailed(action, command, errMsg, hint string, err error) {
	if globalJSON {
		detail := ""
		if err != nil && globalVerbose {
			detail = err.Error()
		}
		_ = outputJSON(JSONEnvelope{Command: command, Status: "failed", Error: errMsg, ErrorDetail: detail, Hint: hintPtr(hint)})
		return
	}
	statusFailed(action, errMsg, hint)
	if globalVerbose && err != nil {
		fmt.Fprintf(os.Stderr, "  detail: %v\n", err)
	}
}
