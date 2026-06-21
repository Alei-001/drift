# Drift

Lightweight version control for creative workers (illustrators, designers, writers). Not Git-compatible.

## Commands

```bash
go build ./...          # Build all
go vet ./...            # Lint
go test ./...           # Tests (none yet)
go run ./cmd/drift/     # Run CLI
```

## Architecture

```
cmd/drift/main.go       → Entry point, calls cli.Execute()
internal/
  cli/root.go           → Cobra commands (currently only: drift init)
  core/                 → Domain types: Blob, Tree, Commit, hash functions
  storage/store.go      → Object store: content-addressable, atomic writes, file locking
```

- Module: `github.com/drift/drift`
- Storage: `.drift/` directory (objects/blobs, objects/trees, commits, refs)
- Hash: pure SHA-256 (no git header), hex-encoded

## Conventions

- Code output and error messages must be in English (no Chinese in `.go` files)
- Docs (`docs/`) are in Chinese — that's intentional
- `reference/go-git/` is a read-only reference, gitignored — do not import from it
- Atomic write pattern: write to `.tmp`, then `os.Rename`
- Blobs are content-addressable: same content → same hash → single stored copy
- Progress tracking: update `docs/progress.md` at phase milestones

## Key Design Decisions

- No packfile or delta compression — each blob stored as-is (MVP trade-off)
- No external filesystem abstraction (uses `os` directly, no billy)
- File lock is in-process only (`sync.Mutex` + lock file indicator)
- Tree entries are sorted by name for deterministic hashing
- Commit hash includes timestamp — same inputs at different times produce different hashes
