package storage

import "errors"

// Sentinel errors for the storage layer. Callers use errors.Is to test for
// specific failure modes; lower layers wrap these with fmt.Errorf("%w", ...)
// to attach context while preserving the sentinel identity.
//
// These mirror the error hierarchy described in architecture.md §6.5.
var (
	// ErrNotFound is returned when a requested object does not exist.
	ErrNotFound = errors.New("drift: not found")

	// ErrAlreadyExists is returned when creating an object that already exists.
	ErrAlreadyExists = errors.New("drift: already exists")

	// ErrPermission is returned when an operation is denied by permissions.
	ErrPermission = errors.New("drift: permission denied")

	// ErrLocked is returned when the storage is locked by another live
	// drift process and the requested operation cannot proceed.
	ErrLocked = errors.New("drift: locked by another process")

	// ErrInvalidRef is returned when a reference name or value is malformed.
	ErrInvalidRef = errors.New("drift: invalid reference")

	// ErrCorrupted is returned when on-disk data fails an integrity check.
	ErrCorrupted = errors.New("drift: data corrupted")

	// ErrUnsupported is returned when a backend does not implement an operation.
	ErrUnsupported = errors.New("drift: unsupported operation")
)
