package porcelain

import "errors"

// Sentinel errors for the porcelain (business) layer. Callers use errors.Is
// to test for specific failure modes; lower layers wrap these with
// fmt.Errorf("...: %w", ...) to attach context while preserving the
// sentinel identity.
var (
	// ErrNothingToSave is returned when a snapshot is requested but the
	// workspace has no changes since the last snapshot.
	ErrNothingToSave = errors.New("nothing to save")

	// ErrBranchNotFound is returned when a referenced branch does not exist.
	ErrBranchNotFound = errors.New("branch not found")

	// ErrBranchAlreadyExists is returned when creating a branch that already
	// exists.
	ErrBranchAlreadyExists = errors.New("branch already exists")

	// ErrSnapshotNotFound is returned when a referenced snapshot does not
	// exist.
	ErrSnapshotNotFound = errors.New("snapshot not found")

	// ErrTagAlreadyExists is returned when creating a tag that already exists.
	ErrTagAlreadyExists = errors.New("tag already exists")
)
