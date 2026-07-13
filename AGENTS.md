# AGENTS.md — drift

## Build & test

```powershell
go build ./...            # all packages
go test ./...             # all tests (~30s)
go test -run TestFoo ./internal/porcelain/   # single test
go test -count=1 ./internal/storage/backends/filesystem/  # skip cache
```

Reproducible release build (injects version metadata consumed by
`drift version` / `drift upgrade`):

```powershell
go build -ldflags "-X github.com/Alei-001/drift/internal/version.Version=v0.1.0 \
  -X github.com/Alei-001/drift/internal/version.Commit=$(git rev-parse --short HEAD) \
  -X github.com/Alei-001/drift/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  ./cmd/drift
```

No Makefile. Lint via `.golangci.yml`. Pure `go` toolchain. CI via GitHub Actions
(`.github/workflows/ci.yml` builds+tests on 3 OSes; `release.yml` publishes
on version tags via GoReleaser + Inno Setup).

## Protobuf codegen

```powershell
protoc --proto_path=internal/core --go_out=internal/core --go_opt=paths=source_relative internal/core/snapshot.proto
protoc --proto_path=internal/core --go_out=internal/core --go_opt=paths=source_relative internal/core/index.proto
```

Generated files live in `internal/core/*.pb.go`. The `--go_opt=paths=source_relative`
flag is **required**: without it protoc creates a nested
`internal/core/github.com/Alei-001/drift/internal/core/` directory and the
generated raw descriptor encodes a stale `go_package`, which panics at init
time (`slice bounds out of range [-1:]`).

Only `SnapshotManifest` and `IndexEntryProto` use protobuf. The snapshot wire
codec lives in `internal/core/snapshot_codec.go` — it calls
`proto.Marshal`/`proto.Unmarshal`, no hand-rolled wire encoding.

## Package boundaries (layer order)

```
cmd/                  → CLI (cobra commands, output formatting) — NO business logic
  drift/              → main binary entry point (cmd/drift/main.go)
internal/             → business implementation (not importable by external projects)
  porcelain/          → business logic (snapshot, branch, restore, lock, watch, gc)
  filetype/           → pluggable type engines (text/image/video/binary), 4 sub-packages
  chunker/            → FastCDC + fixed-size chunking algorithms
  storage/            → Storer interface + shared clone helpers + layout/wire-format constants
    backends/         → storage implementations (physically separated from interface)
      filesystem/     → on-disk .drift/ implementation (prod)
      memory/         → in-memory implementation (tests; prefer in porcelain tests)
    refname/          → reference name validation
    stream/           → chunk streaming helpers (PeekHeader, HashFileContent, …)
  remote/             → remote sync (WebDAV/SMB) — depends on storage + core only,
                       NEVER imports storage/backends/filesystem (uses shared
                       constants from storage/layout.go + storage/chunk_format.go)
  core/               → domain types (Hash, Chunk, Snapshot, FileEntry, Config, …)
  util/               → fsutil, glob, pathutil, format, cache
  version/            → build-time version metadata + self-upgrade (GitHub Releases)
```

Imports: stdlib → third-party → project-internal, blank line between groups.

`internal/` enforces the layer order at the Go level: external projects cannot
import any of the business packages, so the public surface is just the CLI.

## Storage backends

- **Memory**: use `storage/backends/memory.NewMemoryStorage()` for porcelain
  tests (no temp dirs). Thread-safe (internal `sync.RWMutex`).
- **Filesystem**: the real on-disk `.drift/` store. Use
  `backends/filesystem.NewFSStorage(root)`.

Shared layout constants (`ChunksDir`, `SnapshotsDir`, …) and chunk wire-format
constants (`ChunkHeaderSize`, `ChunkFlagCompressed`) live in
`internal/storage/layout.go` and `internal/storage/chunk_format.go`. Both the
filesystem backend and the remote sync package reference these — the remote
package must NOT import `storage/backends/filesystem`.

## Sentinels (use errors.Is, not string matching)

| Package | Sentinels |
|---------|-----------|
| `internal/storage/` | `ErrNotFound`, `ErrAlreadyExists`, `ErrPermission`, `ErrInvalidRef`, `ErrCorrupted`, `ErrUnsupported` |
| `internal/porcelain/` | `ErrLocked`, `ErrNothingToSave`, `ErrBranchNotFound`, `ErrBranchAlreadyExists`, `ErrSnapshotNotFound`, `ErrTagAlreadyExists`, `ErrTagNotFound`, `ErrCannotDeleteCurrentBranch`, `ErrCannotDeleteMain`, `ErrCannotRenameMain`, `ErrAmbiguousID`, `ErrCannotUndo`, `ErrUncommittedChanges` |
| `internal/version/` | `ErrNetwork`, `ErrNoRelease`, `ErrNoAsset` |

Always wrap with `fmt.Errorf("…: %w", err)`. In production code, classify errors with `errors.Is` / `errors.As` — never `strings.Contains(err.Error(), …)`. Test code may use `strings.Contains` on error messages to assert user-facing text.

## Testing rules

- Standard library `testing` only — NO testify, gomega, or external assertion libs.
- Tests verify behavior through public interfaces. NO `reflect` or `unsafe` on
  unexported fields.
- Assert against known-good literals, not recomputed values.
- Naming: `TestFunctionName_Scenario`.
- `TestAcquireWorkspaceLock_StaleLockToctouRace` may fail intermittently (more
  often on Windows due to filesystem timing). This race is a known logic bug in
  lock acquisition (see lock.go TOCTOU window), to be fixed by atomic rename
  replacement — not a platform-specific flake.

## Code conventions (verifiable rules)

1. **Acronyms uppercase**: `ID`, `URL`, `HTTP`, `FS`, `MIME` — never `Id`, `Url`, `Mime`.
2. **Receivers**: 1-2 chars reflecting the type (`fs *FSStorage`, `s *Snapshot`).
3. **Single-method interfaces** end in `-er` (`Chunker`, `Differ`, `Storer`).
4. **Doc comments** on every exported symbol, starting with the name.
5. **Nil guards**: any function returning an interface that can be nil must have
   callers check before calling methods (e.g. `DetectEngine` returns `Engine`, caller
   must handle nil).
6. **Comma-ok type assertions**: always use `if x, ok := v.(T); ok` — never bare.
7. **Defer immediately** after resource acquisition.
8. **Named constants**: no magic numbers. `core.HeaderPeekSize = 512`,
   `storage.MaxSymRefDepth = 8`, `core.DefaultChunkMinSize` etc.
9. **File size ≤ 500 lines, functions ≤ 80 lines.** (Generated `*.pb.go` files are exempt.)
10. **Dedup rule**: identical code in ≥2 files → extract to nearest shared ancestor.
11. **ctx.Err()** in every long-running loop.
12. **Path validation**: all user paths through `pathutil.RelToWorkDir`.
13. **Ref names**: all tag/branch names through `refname.Validate()`.
14. **Protobuf message cloning**: never value-copy a generated proto message
    (`clone := *m`) — it embeds `protoimpl.MessageState` with a `sync.Mutex` and
    `go vet` will flag the lock copy. Use `proto.Clone(m).(*T)` instead.

## Module path

```
module github.com/Alei-001/drift
go 1.25
```

Key deps: `cobra`, `zeebo/blake3`, `klauspost/compress/zstd`, `google.golang.org/protobuf`, `go-cdc-chunkers`, `hashicorp/golang-lru/v2`.

## Reference docs

- `docs/CODE_STANDARDS.md` — full coding conventions (authoritative for style, errors, tests, security)
- `docs/CODE_REVIEW.md` — code review standard: bug definition, severity, fix termination criteria
- `docs/architecture.md` — layered architecture diagram, data model, flow diagrams
- `docs/engine-plugin.md` — guide for adding new filetype engines
