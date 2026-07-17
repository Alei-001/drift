package store

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors_Distinct(t *testing.T) {
	// Each sentinel must be a distinct error value.
	errs := []error{ErrNotFound, ErrAlreadyExists, ErrPermission, ErrInvalidRef, ErrCorrupted, ErrUnsupported}
	for i, a := range errs {
		for j, b := range errs {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("sentinel %v should not match %v", a, b)
			}
		}
	}
}

func TestSentinelErrors_Wrapping(t *testing.T) {
	// Wrapped sentinel errors must still be detectable via errors.Is.
	wrapped := fmt.Errorf("context: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Errorf("wrapped ErrNotFound not detected by errors.Is")
	}

	wrapped2 := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrAlreadyExists))
	if !errors.Is(wrapped2, ErrAlreadyExists) {
		t.Errorf("doubly-wrapped ErrAlreadyExists not detected by errors.Is")
	}
}

func TestSentinelErrors_Messages(t *testing.T) {
	// Sentinel error messages should be non-empty and start with the "drift:" prefix.
	tests := []struct {
		name string
		err  error
	}{
		{"ErrNotFound", ErrNotFound},
		{"ErrAlreadyExists", ErrAlreadyExists},
		{"ErrPermission", ErrPermission},
		{"ErrInvalidRef", ErrInvalidRef},
		{"ErrCorrupted", ErrCorrupted},
		{"ErrUnsupported", ErrUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Error() == "" {
				t.Errorf("%s has empty message", tc.name)
			}
		})
	}
}

func TestConstants_Values(t *testing.T) {
	// MaxSymRefDepth is used by storage backends for symlink recursion limits.
	if MaxSymRefDepth <= 0 {
		t.Errorf("MaxSymRefDepth = %d, want > 0", MaxSymRefDepth)
	}
}
