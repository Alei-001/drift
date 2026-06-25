# AGENTS.md

## Build & Test

```bash
go build -o dist/drift ./cmd/drift/   # compile CLI binary to dist/
go test ./...                         # all tests (~8s)
go test ./internal/cli/...            # CLI tests only (slowest, ~5s)
```

Build output goes to `dist/` (gitignored). No Makefile. No lint config.

## Architecture

Single Go module `github.com/drift/drift`, single binary at `cmd/drift/main.go`. Six internal packages:

| Package | Role |
|---------|------|
| `internal/core` | Object model (Blob/Tree/Commit), SHA-256 hashing, binary codecs (DRIX/DREE/DCMT), walker, diff |
| `internal/storage` | Filesystem store (`.drift/`), atomic writes, OS-level file locking |
| `internal/app` | Application layer — all business logic (Save/Switch/Restore/Diff/Export/etc.) |
| `internal/cli` | Presentation layer — cobra command constructors (`NewXxxCmd(app)`), output formatting |
| `internal/config` | JSON config read/write |
| `internal/worktree` | Working tree operations (staging, WIP, clean) |

Architecture is layered: `cmd/drift` → `cli` (presentation) → `app` (business logic) → `core`/`storage`/`worktree`/`config` (infrastructure). CLI files import only `app/` and `core/` — never `storage`, `worktree`, `config`, or `repo` directly.

`main.go` constructs an `app.App` instance and passes it to `cli.BuildRootCmd(app)`. No global mutable state in CLI — each command receives `*app.App` via closure.

Storage uses content-addressing (pure SHA-256, not Git-compatible). Atomic writes via `tmp + Rename` everywhere.

## Cross-platform

Platform-specific lock implementations: `lock_windows.go` (LockFileEx) and `lock_unix.go` (flock). Paths internally use `/`; convert with `filepath.FromSlash`/`filepath.ToSlash` at filesystem boundaries.

## Testing

**No global mutable state.** Each test creates its own `app.App` via `NewTestHelper(t)` which sets up a temp dir, initializes a store, and constructs an App. No `SetupSharedState()`, no `resetAllFlags()`, no package-level vars.

**Test helpers** live in `cli_test.go`: `TestHelper` struct with `RunAdd`, `RunSave`, `RunStatus`, `RunExport`, etc. Each `RunXxx` method uses `BuildRootCmd(h.App)` + `cmd.SetArgs()` to execute commands. `CaptureOutput` redirects `os.Stdout` to capture command output.

**Tests use real filesystem** — no mocking of `os.ReadFile` etc. Tests create temp dirs, run real `drift init`/`drift save`, then assert file tree state.

## Key conventions

- `.drift/` is the VCS directory (analogous to `.git/`)
- Commit IDs are hash-based (8 hex chars), not sequential version numbers
- Commits are stored by SHA-256 hash as filename; the 8-char ID is a display convenience
- No merge — branches are independent creative exploration lines
- Documentation in `docs/` is project design docs, not user-facing help
- Code output and error messages must be in English (no Chinese in `.go` files)
- Docs (`docs/`) are in Chinese — that's intentional
- `reference/go-git/` is a read-only reference, gitignored — do not import from it
- When implementing similar features, prefer referencing go-git's implementation approach instead of writing from scratch, to improve code quality and consistency with established patterns
- Use standard library when possible (e.g., `encoding/hex` instead of custom hex functions)
- Use codegraph for dependency analysis (`codegraph explore`, `codegraph node`)
