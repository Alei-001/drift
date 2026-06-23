# AGENTS.md

## Build & Test

```bash
go build ./cmd/drift/          # compile CLI binary
go test ./...                  # all tests (~8s)
go test ./internal/cli/...     # CLI tests only (slowest, ~5s)
```

No Makefile. No lint config.

## Architecture

Single Go module `github.com/drift/drift`, single binary at `cmd/drift/main.go`. Four internal packages:

| Package | Role |
|---------|------|
| `internal/core` | Object model (Blob/Tree/Commit), SHA-256 hashing, binary codecs (DRIX/DREE/DCMT), walker, diff |
| `internal/storage` | Filesystem store (`.drift/`), atomic writes, OS-level file locking |
| `internal/cli` | All cobra commands, global mutable state (`sharedDir`/`sharedStore`/`sharedConfig`) |
| `internal/config` | JSON config read/write |

Storage uses content-addressing (pure SHA-256, not Git-compatible). Atomic writes via `tmp + Rename` everywhere.

## Cross-platform

Platform-specific lock implementations: `lock_windows.go` (LockFileEx) and `lock_unix.go` (flock). Paths internally use `/`; convert with `filepath.FromSlash`/`filepath.ToSlash` at filesystem boundaries.

## Testing hazards

**Global mutable state.** `internal/cli` uses three package-level vars (`sharedDir`, `sharedStore`, `sharedConfig`) set by `TestHelper.SetupSharedState()`. Tests change `os.Chdir()` into `T.TempDir()`. Cleanup resets globals and restores cwd.

**Cobra flags are global.** Flag values persist across test cases. Every test must reset flags explicitly — see `resetAllFlags()` in `cli_test.go`. Add new flag resets there when adding new flags.

**Test helpers** live in `cli_test.go`: `TestHelper` struct with `RunAdd`, `RunSave`, `RunStatus`, `RunExport`, etc. `CaptureOutput` redirects `os.Stdout` to capture command output.

**Tests use real filesystem** — no mocking of `os.ReadFile` etc. Tests create temp dirs, run real `drift init`/`drift save`, then assert file tree state.

## Key conventions

- `.drift/` is the VCS directory (analogous to `.git/`)
- Commit IDs use per-branch sequential version numbers (`v1`, `v2`, …), not hashes, for user-facing commands
- Commits are stored by SHA-256 hash as filename; the sequential `v1`/`v2` is a convenience layer
- No merge — branches are independent creative exploration lines
- Documentation in `docs/` is project design docs, not user-facing help
- Code output and error messages must be in English (no Chinese in `.go` files)
- Docs (`docs/`) are in Chinese — that's intentional
- `reference/go-git/` is a read-only reference, gitignored — do not import from it
- When implementing similar features, prefer referencing go-git's implementation approach instead of writing from scratch, to improve code quality and consistency with established patterns
- Use standard library when possible (e.g., `encoding/hex` instead of custom hex functions)
- Use codegraph for dependency analysis (`codegraph explore`, `codegraph node`)
