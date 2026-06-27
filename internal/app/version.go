package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// RepoVersion is the current on-disk repository format version.
// Bump this whenever the .drift/ layout or binary blob format changes.
//
// History:
//
//	1 — Initial format (two-level object dirs, .dre/.dcm extensions, SHA-256)
const RepoVersion = 1

// WriteRepoVersion persists the current format version to .drift/version.
func WriteRepoVersion(driftDir string) error {
	p := filepath.Join(driftDir, "version")
	return os.WriteFile(p, []byte(strconv.Itoa(RepoVersion)+"\n"), 0644)
}

// ReadRepoVersion returns the format version stored in .drift/version.
// Returns 0 when no version file exists (pre-versioning repository).
func ReadRepoVersion(driftDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(driftDir, "version"))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("corrupt .drift/version: %w", err)
	}
	return v, nil
}

// CheckRepoVersion compares the repo format version against the binary.
// It returns:
//
//	outdated=true  — repo version < RepoVersion (needs upgrade)
//	outdated=false — catch up or pre-versioning (v0 treated as v1)
//
// An error is returned only when the repo version exceeds RepoVersion
// (i.e. the binary is too old to read this repo).
func CheckRepoVersion(driftDir string) (outdated bool, err error) {
	v, err := ReadRepoVersion(driftDir)
	if err != nil {
		return false, err
	}
	if v == 0 {
		return false, nil // pre-versioning — upgrade handles it
	}
	if v > RepoVersion {
		return false, fmt.Errorf(
			"repository format v%d is newer than this drift binary (v%d). Please upgrade the drift binary first.",
			v, RepoVersion,
		)
	}
	return v < RepoVersion, nil
}

// WarnIfOutdated prints a one-time warning to stderr when the repository
// needs a format upgrade. It also silently marks pre-versioning repos as v1
// so that future format migrations can detect them.
func (a *App) WarnIfOutdated() {
	if a.store == nil || !a.store.IsInitialized() {
		return
	}

	// Auto-mark pre-versioning repos (no .drift/version file) as v1.
	v, err := ReadRepoVersion(a.store.DriftDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "drift: version check: %v\n", err)
		return
	}
	if v == 0 {
		_ = WriteRepoVersion(a.store.DriftDir())
		v = 1
	}

	if v > RepoVersion {
		fmt.Fprintf(os.Stderr,
			"drift: repository format v%d is newer than this binary (v%d). Please upgrade the drift binary.\n",
			v, RepoVersion)
		return
	}
	if v < RepoVersion {
		fmt.Fprintf(os.Stderr,
			"drift: repository format is v%d, current binary supports v%d. Run 'drift upgrade' to migrate.\n",
			v, RepoVersion)
	}
}
