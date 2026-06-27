package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// Migration describes a single format-version bump.
// Each migration advances the repo from (From) to (To).
// Run receives the locked store and must be idempotent —
// the upgrade command may be interrupted and re-run.
type Migration struct {
	From int
	To   int
	Desc string
	Run  func(a *App) error
}

// migrations holds every known migration, ordered by From.
// Append new entries at the bottom.
var migrations = []Migration{
	// Example (for use when format changes):
	// {From: 1, To: 2, Desc: "add packfile support", Run: migrateV1ToV2},
}

// NeedsUpgrade returns true when the repository needs a format upgrade.
func (a *App) NeedsUpgrade() bool {
	outdated, _ := CheckRepoVersion(a.store.DriftDir())
	return outdated
}

// UpgradeResult summarizes an upgrade run.
type UpgradeResult struct {
	From        int  // starting format version (0 means pre-versioning)
	To          int  // ending format version
	AlreadyDone bool // nothing to do
	DryRun      bool
}

// Upgrade brings the repository from its current format version to RepoVersion.
// When DryRun is true, migrations are listed but not executed.
// The store is locked during each migration.
func (a *App) Upgrade(dryRun bool) (*UpgradeResult, error) {
	driftDir := a.store.DriftDir()
	current, err := ReadRepoVersion(driftDir)
	if err != nil {
		return nil, fmt.Errorf("read version: %w", err)
	}

	result := &UpgradeResult{From: current, To: RepoVersion, DryRun: dryRun}

	// Pre-versioning repo: mark it as v1 (current format has always been v1).
	if current == 0 {
		if !dryRun {
			if err := WriteRepoVersion(driftDir); err != nil {
				return nil, fmt.Errorf("write version: %w", err)
			}
		}
		result.To = RepoVersion
		return result, nil
	}

	if current >= RepoVersion {
		result.AlreadyDone = true
		return result, nil
	}

	for _, m := range migrations {
		if m.From < current {
			continue
		}
		if m.From >= RepoVersion {
			break
		}
		if dryRun {
			continue
		}
		fmt.Printf("Running migration: %s (v%d → v%d)\n", m.Desc, m.From, m.To)
		if err := a.store.WithLock(func() error {
			if innerErr := m.Run(a); innerErr != nil {
				return fmt.Errorf("migration v%d→v%d: %w", m.From, m.To, innerErr)
			}
			// Bump version to mark this migration as complete.
			// Re-read current version in case another process raced us.
			v, verr := ReadRepoVersion(driftDir)
			if verr != nil {
				return fmt.Errorf("read version after migration: %w", verr)
			}
			if v <= m.From {
				// Create a temporary version file pointing to the new version.
				// We don't use WriteRepoVersion because it always writes the
				// binary's current RepoVersion. Here we need to write an
				// intermediate version.
				return writeVersion(driftDir, m.To)
			}
			return nil
		}); err != nil {
			return result, err
		}
		result.To = m.To
		fmt.Printf("Repository at format version %d\n", m.To)
	}

	// After all migrations, write the final RepoVersion to catch up.
	if !dryRun {
		if err := WriteRepoVersion(driftDir); err != nil {
			return result, fmt.Errorf("write final version: %w", err)
		}
		result.To = RepoVersion
	}

	return result, nil
}

// writeVersion writes an arbitrary version number (not necessarily RepoVersion).
// Used by migrations to checkpoint intermediate versions.
func writeVersion(driftDir string, v int) error {
	return os.WriteFile(
		filepath.Join(driftDir, "version"),
		[]byte(fmt.Sprintf("%d\n", v)),
		0644,
	)
}
