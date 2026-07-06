package refname

import (
	"errors"
	"testing"

	"github.com/your-org/drift/internal/storage"
)

func TestValidate_Valid(t *testing.T) {
	valid := []string{
		"HEAD",
		"heads/main",
		"heads/feature-branch",
		"tags/v1.0",
		"tags/release-2024",
		"heads/feature_branch",
	}
	for _, name := range valid {
		if err := Validate(name); err != nil {
			t.Errorf("Validate(%q) returned error: %v", name, err)
		}
	}
}

func TestValidate_Empty(t *testing.T) {
	err := Validate("")
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for empty name, got %v", err)
	}
}

func TestValidate_ControlChars(t *testing.T) {
	invalid := []string{
		"foo\x00bar",
		"foo\nbar",
		"foo\x1bbar",
		"foo\x7fbar",
	}
	for _, name := range invalid {
		err := Validate(name)
		if !errors.Is(err, storage.ErrInvalidRef) {
			t.Errorf("expected ErrInvalidRef for %q, got %v", name, err)
		}
	}
}

func TestValidate_Space(t *testing.T) {
	err := Validate("foo bar")
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for name with space, got %v", err)
	}
}

func TestValidate_BackslashAndColon(t *testing.T) {
	invalid := []string{
		"foo\\bar",
		"foo:bar",
	}
	for _, name := range invalid {
		err := Validate(name)
		if !errors.Is(err, storage.ErrInvalidRef) {
			t.Errorf("expected ErrInvalidRef for %q, got %v", name, err)
		}
	}
}

func TestValidate_DotDot(t *testing.T) {
	err := Validate("foo..bar")
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for '..' in name, got %v", err)
	}
}

func TestValidate_LeadingDash(t *testing.T) {
	err := Validate("-branch")
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for leading dash, got %v", err)
	}
}

func TestValidate_LeadingSlash(t *testing.T) {
	err := Validate("/branch")
	if !errors.Is(err, storage.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for leading slash, got %v", err)
	}
}

func TestIsWindowsReservedName(t *testing.T) {
	reserved := []string{
		"con", "aux", "nul", "prn",
		"com1", "com2", "com9",
		"lpt1", "lpt2", "lpt9",
		"com0", "lpt0", // modern Windows
		// IsWindowsReservedName only inspects name[3], so multi-digit
		// suffixes like "com10" still match because name[3]=='1'.
		"com10", "lpt12",
	}
	for _, name := range reserved {
		if !IsWindowsReservedName(name) {
			t.Errorf("IsWindowsReservedName(%q) = false, want true", name)
		}
	}

	notReserved := []string{
		"main", "feature", "v1.0",
		"compute", // 4th char 'p' is not a digit
		"lpt",     // too short
		"com",     // too short
		// IsWindowsReservedName is case-sensitive; the caller (Validate)
		// lowercases the input first, so uppercase variants are NOT
		// recognised by this function directly.
		"CON", "AUX", "NUL",
		"Com1", "LPT9",
	}
	for _, name := range notReserved {
		if IsWindowsReservedName(name) {
			t.Errorf("IsWindowsReservedName(%q) = true, want false", name)
		}
	}
}
