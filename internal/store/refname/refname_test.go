package refname

import (
	"errors"
	"testing"

	"github.com/Alei-001/drift/internal/store"
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
	if !errors.Is(err, store.ErrInvalidRef) {
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
		if !errors.Is(err, store.ErrInvalidRef) {
			t.Errorf("expected ErrInvalidRef for %q, got %v", name, err)
		}
	}
}

func TestValidate_Space(t *testing.T) {
	err := Validate("foo bar")
	if !errors.Is(err, store.ErrInvalidRef) {
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
		if !errors.Is(err, store.ErrInvalidRef) {
			t.Errorf("expected ErrInvalidRef for %q, got %v", name, err)
		}
	}
}

func TestValidate_DotDot(t *testing.T) {
	err := Validate("foo..bar")
	if !errors.Is(err, store.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for '..' in name, got %v", err)
	}
}

func TestValidate_LeadingDash(t *testing.T) {
	err := Validate("-branch")
	if !errors.Is(err, store.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for leading dash, got %v", err)
	}
}

func TestValidate_LeadingSlash(t *testing.T) {
	err := Validate("/branch")
	if !errors.Is(err, store.ErrInvalidRef) {
		t.Errorf("expected ErrInvalidRef for leading slash, got %v", err)
	}
}

func TestValidate_ReservedKeyword(t *testing.T) {
	// "head" is reserved as a snapshot reference keyword and cannot be used
	// as a branch or tag name (base name). The check is case-insensitive.
	// "HEAD" (uppercase, no prefix) is the system ref and remains valid.
	invalid := []string{"heads/head", "tags/head", "heads/Head", "heads/HEAD"}
	for _, name := range invalid {
		err := Validate(name)
		if !errors.Is(err, store.ErrInvalidRef) {
			t.Errorf("Validate(%q) = %v, want ErrInvalidRef (reserved keyword)", name, err)
		}
	}
}

func TestIsWindowsReservedName(t *testing.T) {
	reserved := []string{
		"con", "aux", "nul", "prn",
		"com1", "com2", "com9",
		"lpt1", "lpt2", "lpt9",
		"com0", "lpt0", // modern Windows
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
		// Multi-digit suffixes are NOT reserved on Windows: only COM0-9
		// and LPT0-9 (exactly 4 chars) are reserved.
		"com10", "lpt12", "com99",
	}
	for _, name := range notReserved {
		if IsWindowsReservedName(name) {
			t.Errorf("IsWindowsReservedName(%q) = true, want false", name)
		}
	}
}

func TestIsWindowsReservedName_Uppercase(t *testing.T) {
	uppercase := []string{
		"CON", "AUX", "NUL", "PRN",
		"COM0", "COM1", "COM9",
		"LPT0", "LPT1", "LPT9",
	}
	for _, name := range uppercase {
		if !IsWindowsReservedName(name) {
			t.Errorf("IsWindowsReservedName(%q) = false, want true", name)
		}
	}
}

func TestIsWindowsReservedName_MixedCase(t *testing.T) {
	mixed := []string{
		"Con", "Aux", "Nul", "Prn",
		"Com1", "Lpt9",
		"cOn", "aUx", "nUl", "pRn",
	}
	for _, name := range mixed {
		if !IsWindowsReservedName(name) {
			t.Errorf("IsWindowsReservedName(%q) = false, want true", name)
		}
	}
}
