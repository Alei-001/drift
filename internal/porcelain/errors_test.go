package porcelain

import (
	"errors"
	"fmt"
	"testing"
)

// TestSentinelErrors_Distinct verifies that each sentinel error is a distinct
// value: errors.Is must not cross-match any two of them, otherwise callers
// using errors.Is to discriminate failure modes would get false positives.
func TestSentinelErrors_Distinct(t *testing.T) {
	all := []error{
		ErrNothingToSave,
		ErrBranchNotFound,
		ErrBranchAlreadyExists,
		ErrSnapshotNotFound,
		ErrAmbiguousID,
		ErrTagAlreadyExists,
		ErrCannotDeleteCurrentBranch,
		ErrCannotDeleteMain,
		ErrCannotRenameMain,
	}
	for i, a := range all {
		for j, b := range all {
			if i == j {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("sentinels must be distinct: errors.Is(%v, %v) = true (i=%d j=%d)", a, b, i, j)
			}
		}
	}
}

// TestSentinelErrors_WrappingPreservesIdentity verifies that wrapping a
// sentinel with fmt.Errorf("...: %w", sentinel) preserves its identity so
// callers can still detect it via errors.Is. This is the contract documented
// in errors.go and relied on by the storage layer.
func TestSentinelErrors_WrappingPreservesIdentity(t *testing.T) {
	all := []error{
		ErrNothingToSave,
		ErrBranchNotFound,
		ErrBranchAlreadyExists,
		ErrSnapshotNotFound,
		ErrAmbiguousID,
		ErrTagAlreadyExists,
		ErrCannotDeleteCurrentBranch,
		ErrCannotDeleteMain,
		ErrCannotRenameMain,
	}
	for _, sentinel := range all {
		wrapped := fmt.Errorf("operation failed: %w", sentinel)
		if !errors.Is(wrapped, sentinel) {
			t.Errorf("errors.Is failed to identify wrapped sentinel: %v", sentinel)
		}
	}
}

// TestSentinelErrors_NonEmptyMessages verifies every sentinel has a non-empty
// human-readable message. Empty messages would make error logs useless.
func TestSentinelErrors_NonEmptyMessages(t *testing.T) {
	all := []struct {
		name string
		err  error
	}{
		{"ErrNothingToSave", ErrNothingToSave},
		{"ErrBranchNotFound", ErrBranchNotFound},
		{"ErrBranchAlreadyExists", ErrBranchAlreadyExists},
		{"ErrSnapshotNotFound", ErrSnapshotNotFound},
		{"ErrAmbiguousID", ErrAmbiguousID},
		{"ErrTagAlreadyExists", ErrTagAlreadyExists},
		{"ErrCannotDeleteCurrentBranch", ErrCannotDeleteCurrentBranch},
		{"ErrCannotDeleteMain", ErrCannotDeleteMain},
		{"ErrCannotRenameMain", ErrCannotRenameMain},
	}
	for _, tc := range all {
		if tc.err.Error() == "" {
			t.Errorf("%s has empty message", tc.name)
		}
	}
}
