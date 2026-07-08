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

	// ErrAmbiguousID is returned when an id:<prefix> reference matches more
	// than one snapshot. The wrapped error message lists the matching
	// snapshots so callers can surface them to the user.
	ErrAmbiguousID = errors.New("ambiguous snapshot ID prefix")

	// ErrTagAlreadyExists is returned when creating a tag that already exists.
	ErrTagAlreadyExists = errors.New("tag already exists")

	// ErrTagNotFound is returned when a referenced tag does not exist.
	ErrTagNotFound = errors.New("tag not found")

	// ErrCannotUndo is returned when UndoLastSave is called but HEAD is
	// already at the initial snapshot (no previous snapshot to revert to).
	ErrCannotUndo = errors.New("cannot undo: already at initial snapshot")

	// ErrUncommittedChanges is returned when an operation that would lose
	// workspace changes is attempted (e.g. undo with a dirty workspace, or
	// switch --no-autosave with a dirty workspace).
	ErrUncommittedChanges = errors.New("uncommitted changes would be lost")

	// ErrCannotDeleteCurrentBranch is returned when attempting to delete the
	// currently checked-out branch.
	ErrCannotDeleteCurrentBranch = errors.New("cannot delete the current branch")

	// ErrCannotDeleteMain is returned when attempting to delete the 'main'
	// branch, which is protected.
	ErrCannotDeleteMain = errors.New("cannot delete 'main'")

	// ErrCannotRenameMain is returned when attempting to rename the 'main'
	// branch, which is protected.
	ErrCannotRenameMain = errors.New("cannot rename 'main'")
)
