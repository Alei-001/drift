# Drift Architecture Refactoring Tasks

## Overview

7 phases, split into individually verifiable tasks.

**Key constraint**: each task must leave the project in a buildable state (`go build ./...` must pass).

## Transition Strategy

Phases P4-P5 use a "parallel coexistence → gradual retirement" strategy:

- P4 creates `*_new.go` files alongside existing `.go` files
- New constructors (`NewXxxCmd`) coexist with old package-level vars (`xxxCmd`)
- Old `init()` functions still register commands on old `rootCmd` (never executed)
- New commands registered via `BuildRootCmd(app)` (actually used)
- P5 moves old `.go` files to `.wastebasket/cli/`, then renames `*_new.go` → `*.go`
- `.wastebasket/` at repo root preserves old code as reference during development

This avoids the "chicken-and-egg" problem where removing a command variable breaks test files that still reference it. Old code stays accessible in `.wastebasket/` until verification is complete.

**After full verification** (all tests pass, binary works), `.wastebasket/` can be deleted.

## `.wastebasket/` Directory Structure

```
.wastebasket/
├── cli/                  # old command files (moved in P5)
│   ├── add.go
│   ├── diff.go
│   ├── ... (18 more)
│   └── root.go
├── repo/                 # old repo package (moved in P5)
│   ├── repo.go
│   ├── save.go
│   └── ... (6 more)
└── core/
    └── progress.go       # dead code (moved in P5)
```

`.wastebasket/` is git-ignored (add to `.gitignore`).

## Phase Order

```
P1 ──→ P2 ──→ P3 ──→ P4 ──→ P5 ──→ P6 ──→ P7
(all new)       (framework)  (new cmds) (retire old) (tests) (polish)
```

**Before P5**: record output snapshots from the pre-refactor binary — see
`07-phase7-polish.md` §7.3a. Once P5 retires the old code, the golden binary
can no longer be built, so snapshots must be captured while the old `cli/`
still compiles.

| Phase | Focus | Files |
|-------|-------|-------|
| [P1](01-phase1-app-skeleton.md) | App package skeleton | 15 new |
| [P2](02-phase2-app-implementation.md) | App method implementations | 15 modify |
| [P3](03-phase3-cli-framework.md) | BuildRootCmd + main.go rewrite | 1 new, 1 modify |
| [P4](04-phase4-command-rewrite.md) | Command constructors (_new.go) | 20 new, 1 modify |
| [P5](05-phase5-cleanup.md) | Retire old code to .wastebasket | 21 move, 21 rename |
| [P6](06-phase6-test-migration.md) | Test updates for new architecture | ~14 modify |
| [P7](07-phase7-polish.md) | Chinese text, go vet, snapshot regression, final smoke test | ~5 modify |

## Verification Gate

Before each commit, run:

```bash
go build ./...   &&   go test ./...
```

---

## Error Handling Conventions

These conventions apply to ALL code in this refactoring. The refactoring itself is the opportunity to enforce them consistently.

### C1: Never silently swallow non-cleanup errors

`_ = expr` is allowed ONLY in the following cases:

| Allowed | Pattern |
|---------|---------|
| Temp file cleanup after error | `_ = os.Remove(tmpPath)` in error-recovery path |
| Stat check follow-up | `info, _ := os.Stat(path)` when the only purpose is existence check |
| Explicitly documented non-fatal | Must have a `// Non-fatal:` comment explaining why |

**Prohibited**:

```go
currentBranch, _ := store.GetRef("HEAD")  // BAD — corruption silently hidden
_ = store.LoadIndex(&idx)                  // BAD — failure silently ignored
_ = worktree.DeleteWIP(store, branch)      // BAD — undo may produce inconsistent state
```

### C2: Use `fmt.Errorf` with `%w` for wrapping

```go
if err := store.LoadIndex(&idx); err != nil {
    return fmt.Errorf("failed to load index: %w", err)
}
```

Do NOT use `errors.Wrap` or any external error library. `fmt.Errorf("%w")` is the standard-library way.

### C3: Error messages start lowercase, no trailing period

```go
return fmt.Errorf("commit not found: %s", hash)     // GOOD
return fmt.Errorf("Commit not found: %s.", hash)    // BAD
```

### C4: Error return path for undo/rollback

When an operation modifies state and the subsequent step fails, ensure undo doesn't silently skip:

```go
// BAD — undo produces inconsistent state
for _, change := range last.RefChanges {
    _ = r.Store.DeleteRef(change.Ref)  // failure hidden
}

// GOOD — accumulate errors
var errs []error
for _, change := range last.RefChanges {
    if err := r.Store.DeleteRef(change.Ref); err != nil {
        errs = append(errs, err)
    }
}
if len(errs) > 0 {
    return errors.Join(errs...)
}
```

### C5: Functions that always return (success, error) use that pattern

```go
func (a *App) Add(paths []string) (int, error)  // count + error
func (a *App) Save(msg string, opts SaveOptions) (*SaveResult, error)  // result + error
```

Don't return `(nil, nil)` for "no result, no error" — use `(zeroValue, nil)`.
