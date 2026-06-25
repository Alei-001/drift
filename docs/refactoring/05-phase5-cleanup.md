# Phase 5: Retire Old Code

## Goal

Move all old code to `.wastebasket/` for reference, then rename `*_new.go` files to their final names. Remove global mutable state from the active codebase.

After this phase, the project uses only the new architecture. **Tests will break** — test migration is Phase 6.

Old code stays accessible in `.wastebasket/` as coding reference. After full verification (P7), `.wastebasket/` can be deleted.

## Verification

```bash
go build ./...              # must pass
go test ./...               # expected to FAIL (tests reference old global vars)
```

## Tasks

### 5.1 — Create .wastebasket/ base directory

```bash
mkdir -p .wastebasket
```

Ensure `.gitignore` contains `.wastebasket/`.

Do NOT pre-create `.wastebasket/cli`, `.wastebasket/repo`, `.wastebasket/core` — subdirectories are created implicitly by the move operations below.

### 5.2 — Move old CLI command files to .wastebasket/cli/

> **Use plain `mv`, NOT `git mv`.** `.wastebasket/` is gitignored, and `git mv`
> refuses to move tracked files into an ignored path. Plain `mv` makes git see
> this as "delete old file + untracked new file", which is the intended effect
> (the deletion is committed; the archived copy stays local-only).

```bash
mkdir -p .wastebasket/cli
mv internal/cli/add.go       .wastebasket/cli/
mv internal/cli/save.go      .wastebasket/cli/
mv internal/cli/unstage.go   .wastebasket/cli/
mv internal/cli/diff.go      .wastebasket/cli/
mv internal/cli/export.go    .wastebasket/cli/
mv internal/cli/history.go   .wastebasket/cli/
mv internal/cli/log.go       .wastebasket/cli/
mv internal/cli/status.go    .wastebasket/cli/
mv internal/cli/switch.go    .wastebasket/cli/
mv internal/cli/branch.go    .wastebasket/cli/
mv internal/cli/name.go      .wastebasket/cli/
mv internal/cli/rm.go        .wastebasket/cli/
mv internal/cli/mv.go        .wastebasket/cli/
mv internal/cli/wip.go       .wastebasket/cli/
mv internal/cli/clean.go     .wastebasket/cli/
mv internal/cli/clone.go     .wastebasket/cli/
mv internal/cli/config.go    .wastebasket/cli/
mv internal/cli/sync.go      .wastebasket/cli/
mv internal/cli/restore.go   .wastebasket/cli/
mv internal/cli/root.go      .wastebasket/cli/
```

Files that stay in `internal/cli/`:
- `*_new.go` files (20 command constructors)
- `root_new.go` — `BuildRootCmd()`
- `cli_test.go` — test helper
- All `*_test.go` files
- `confirm.go`, `color.go` — unchanged utilities

### 5.3 — Rename *_new.go → *.go

> **Here `git mv` IS correct** — both source (`*_new.go`) and destination
> (`*.go`) live inside `internal/cli/`, which is NOT gitignored. This preserves
> git history for the rewritten files. (Contrast with 5.2/5.4/5.5, where the
> target is gitignored and `git mv` would fail.)

```bash
cd internal/cli/

# Use git mv to preserve history
for f in *_new.go; do
    git mv "$f" "${f%_new.go}.go"
done
```

This renames: `add_new.go → add.go`, `save_new.go → save.go`, etc.

After this step, `internal/cli/` contains clean `.go` files with only the new constructor-based architecture.

### 5.4 — Move old repo/ to .wastebasket/repo/

> **Use plain `mv`, NOT `git mv`** — same reason as 5.2 (target is gitignored).

```bash
# .wastebasket/repo/ does NOT exist yet (we only created .wastebasket/)
# so this RENAMES internal/repo → .wastebasket/repo
mv internal/repo .wastebasket/repo
```

### 5.5 — Move dead code to .wastebasket/core/

> **Use plain `mv`** — target is gitignored.

```bash
mkdir -p .wastebasket/core
mv internal/core/progress.go   .wastebasket/core/
```

### 5.6 — Remove unused diff functions from core/diff.go

Edit `internal/core/diff.go` — remove:
- `DiffEditScriptToUnified()` function (line ~231)
- `DiffCountChanges()` function (line ~243)

These are small pure functions with zero callers. No need to preserve in `.wastebasket/`.

### 5.7 — Clean up stale imports in CLI source files

After moving old files out, some `internal/cli/` source files may still contain stale imports. Run:

```bash
go build ./...              # find compilation errors
goimports -w internal/cli/  # auto-fix imports
```

Verify:
```bash
# No storage imports in active CLI code
rg '"github.com/drift/drift/internal/storage"' internal/cli/ --glob '!*_test.go'
# Should return empty

# No worktree imports in active CLI code
rg '"github.com/drift/drift/internal/worktree"' internal/cli/ --glob '!*_test.go'
# Should return empty

# No config imports in active CLI code
rg '"github.com/drift/drift/internal/config"' internal/cli/ --glob '!*_test.go'
# Should return empty
```

### 5.8 — Final build verification

```bash
go build ./...
```

Must pass. No reference to `sharedStore`, `sharedRepo`, `sharedDir`, `sharedConfig`, `internal/repo`, `internal/core/progress.go`.

## .wastebasket/ Retention Policy

- Keep `.wastebasket/` throughout P6 (test migration) and P7 (polish)
- Used as reference when rewriting tests or debugging
- Delete after P7 smoke test passes (optional — can keep longer if needed)

## Deliverables

- `.wastebasket/cli/` — 20 old command files preserved for reference
- `.wastebasket/repo/` — 8 old repo files preserved
- `.wastebasket/core/progress.go` — dead code preserved
- `internal/cli/` — clean, no old files, no global state
- `internal/repo/` — deleted
- `internal/core/progress.go` — deleted
- `go build ./...` passes
