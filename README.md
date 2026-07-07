# drift

Version control for creators — a content-addressed, chunk-deduplicated version
control system designed for writers, illustrators, and designers. Unlike Git,
which treats every file as opaque bytes, drift understands rich media (images,
PSD, video) and stores only what actually changed.

> Status: Phase 1–3 complete (local core + branches + filetype engines). GUI
> and remote sync are planned phases; see [docs/roadmap.md](docs/roadmap.md).

## Why drift?

| Pain point with Git | drift answer |
|---|---|
| 200 MB PSD changes by one layer → store the whole file again | FastCDC content-defined chunking stores only the changed blocks |
| Staging area / commits / merge — a programmer's mental model | `save` captures all changes automatically; no staging, no merge |
| Diff shows raw bytes for binary files | Pluggable filetype engines (text/image/video/binary) with format-aware diff and previews |
| No visual timeline | Thumbnails and metadata are generated at save time (Phase 3), preparing for the GUI timeline (Phase 4) |
| Branches mean merge conflicts | Branches are pure forks for experimentation; user merges manually |

## Features

- **Content-addressed storage** — BLAKE3 hashes verify integrity and dedupe
  automatically across snapshots and branches.
- **CDC chunking** — FastCDC for variable-size content-defined chunks, with a
  fixed-size fallback for very large files (>100 MB). zstd compression on top.
- **No staging area** — `drift save` captures the working tree as-is. Authors
  think about their work, not about indexes.
- **Branches without merge** — create experimental branches, switch freely,
  restore from anywhere. No merge conflicts, ever.
- **Filetype engines** — text (unified diff), image (metadata + preview),
  video (metadata), binary (fallback). New engines plug in via a registry.
- **Automatic watches** — `drift watch on` auto-saves on file change, with
  auto-saves hidden from `log` by default.
- **Single binary** — one static Go binary for macOS / Windows / Linux. No
  runtime, no daemons to install.

## Install

```powershell
go install github.com/your-org/drift/cmd/drift@latest
```

Or build from source:

```powershell
git clone <repo-url>
cd drift
go build -o drift ./cmd/drift
```

Requires Go 1.24+.

## Quick start

```powershell
# Initialize a project
cd my-novel
drift init

# Save a snapshot
drift save -m "Chapter 1 draft"

# See what changed since the last save
drift status

# Browse history (current branch chain)
drift log

# Try an experimental direction
drift branch create rewrite-ending
drift switch rewrite-ending
# ... edit files ...
drift save -m "Alt ending v1"

# Switch back; the rewrite is preserved on its own branch
drift switch main

# Inspect a specific snapshot's file changes
drift log --detail @id:12ab

# Restore the workspace to a previous snapshot (auto-backs up first)
drift restore @id:12ab
```

## Commands

| Command | Purpose |
|---|---|
| `drift init` | Create `.drift/` repository |
| `drift save [-m <msg>] [--tag <name>]` | Save a snapshot of all changes |
| `drift status` | Show added / modified / deleted files |
| `drift log [--branch <name>] [--all] [--limit <n>]` | Browse snapshot history |
| `drift show <version> [<file>]` | List files in a snapshot, or display a file's content |
| `drift diff <v1> <v2>` | Diff two snapshots (file list or unified diff) |
| `drift restore <version>` | Restore the workspace to a snapshot (backs up first) |
| `drift undo` | Undo the last save |
| `drift branch {list,create,delete,rename}` | Manage branches |
| `drift switch <branch>` | Switch to a branch (optionally create with `-c`) |
| `drift tag {list,add,delete,rename}` | Manage tags |
| `drift watch {on,off,status,pause,resume}` | Background auto-save daemon |
| `drift check` | Verify `.drift/` storage integrity |
| `drift gc` | Remove unreachable snapshots and chunks |
| `drift config {get,set,list}` | View and modify configuration |

### Version references

Commands that take a `<version>` accept:

- `@head` — current HEAD snapshot
- `@id:<hash-prefix>` — match by hash prefix (≥ 4 chars)
- `@tag:<name>` — resolve via tag
- `@branch:<name>` — resolve via branch head
- `<bare-name>` — shorthand for `@branch:<bare-name>`

## Project layout

```
cmd/                  CLI entry points (cobra commands) — no business logic
  drift/              main binary
internal/             business implementation (not importable externally)
  porcelain/          business logic: snapshot, branch, restore, lock, watch, gc
  filetype/           pluggable type engines (text/image/video/binary)
  chunker/            FastCDC + fixed-size chunking
  storage/            Storer interface + shared helpers
    backends/         filesystem (prod) and memory (tests) implementations
    refname/          branch / tag name validation
    stream/           chunk streaming helpers
  core/               domain types: Hash, Chunk, Snapshot, FileEntry, Config, ...
  util/               fsutil, glob, pathutil, format, cache
docs/                 design and reference docs
```

See [AGENTS.md](AGENTS.md) for the full layering rules and conventions.

## Documentation

- [docs/prd.md](docs/prd.md) — product requirements
- [docs/roadmap.md](docs/roadmap.md) — development roadmap
- [docs/cli-design.md](docs/cli-design.md) — CLI design and output formats
- [docs/architecture.md](docs/architecture.md) — layered architecture and data model
- [docs/CODE_STANDARDS.md](docs/CODE_STANDARDS.md) — coding conventions (authoritative)
- [docs/CODE_REVIEW.md](docs/CODE_REVIEW.md) — code review standard
- [docs/engine-plugin.md](docs/engine-plugin.md) — guide for adding new filetype engines

## Building and testing

```powershell
go build ./...            # build all packages
go test ./...             # run all tests
go test -run TestFoo ./internal/porcelain/   # single test
```

No Makefile, no CI workflows, no lint config — pure `go` toolchain.

### Protobuf codegen

Generated files live in `internal/core/*.pb.go`. Regenerate with:

```powershell
protoc --proto_path=internal/core --go_out=internal/core --go_opt=paths=source_relative internal/core/snapshot.proto
protoc --proto_path=internal/core --go_out=internal/core --go_opt=paths=source_relative internal/core/index.proto
```

The `--go_opt=paths=source_relative` flag is **required** (see AGENTS.md).

## Key dependencies

- [cobra](https://github.com/spf13/cobra) — CLI framework
- [zeebo/blake3](https://github.com/zeebo/blake3) — content hashing
- [klauspost/compress](https://github.com/klauspost/compress) — zstd compression
- [google.golang.org/protobuf](https://pkg.go.dev/google.golang.org/protobuf) — snapshot wire format
- [PlakarKorp/go-cdc-chunkers](https://github.com/PlakarKorp/go-cdc-chunkers) — FastCDC implementation
- [hashicorp/golang-lru/v2](https://github.com/hashicorp/golang-lru) — chunk and preview caches

## License

See the project repository for license information.
