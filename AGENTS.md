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
  cli/                  → Cobra commands: init, add, status, reset, save, list
  core/                 → Domain types: Blob, Tree, Commit, Index, hash functions
  storage/store.go      → Object store: content-addressable, atomic writes, file locking
```

- Module: `github.com/drift/drift`
- Storage: `.drift/` directory (objects/blobs, objects/trees, commits, refs)
- Hash: pure SHA-256 (no git header), hex-encoded

## Binary Formats

All persistent data uses binary formats for performance:

| Format | Magic | Extension | Purpose |
|--------|-------|-----------|---------|
| DRIX | `DRIX` | (none) | Index (staging area) |
| DREE | `DREE` | `.dre` | Tree objects |
| DCMT | `DCMT` | `.dcm` | Commit objects |

## Conventions

- Code output and error messages must be in English (no Chinese in `.go` files)
- Docs (`docs/`) are in Chinese — that's intentional
- `reference/go-git/` is a read-only reference, gitignored — do not import from it
- Use standard library when possible (e.g., `encoding/hex` instead of custom hex functions)
- Use codegraph for dependency analysis
- Atomic write pattern: write to `.tmp`, then `os.Rename`
- Blobs are content-addressable: same content → same hash → single stored copy
- Progress tracking: update `docs/progress.md` at phase milestones

## Key Design Decisions

- No packfile or delta compression — each blob stored as-is (MVP trade-off)
- No external filesystem abstraction (uses `os` directly, no billy)
- File lock is in-process only (`sync.Mutex` + lock file indicator)
- Tree entries are sorted by type (directories first) then name for deterministic hashing
- Tree is recursive — each directory is an independent Tree object
- Commit hash includes timestamp — same inputs at different times produce different hashes
- All objects use binary format (DRIX/DREE/DCMT) for performance
