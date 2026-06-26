<p align="center"><img src="assets/icon.png" alt="Drift" width="96"></p>

# Drift

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![中文文档](https://img.shields.io/badge/README-中文-red.svg)](docs/README.zh-CN.md)

A lightweight version control tool for creative workers — illustrators, designers, novelists, screenwriters. Manage multiple versions of creative work without learning Git.

> **README in Chinese:** [docs/README.zh-CN.md](docs/README.zh-CN.md)

## Why Drift?

Creative workers manage versions by hand today — folders named `final`, `final_v2`, `final_really_final`, `final_really_final_client_picked_this`. Drift replaces that chaos with a simple mental model:

```
save a version  →  go back to any version  →  export for delivery
```

No staging area jargon. No merge conflicts. No Git concepts to learn.

## Features

- **Simple** — 10-minute onboarding, no Git knowledge required
- **Staging preview** — see exactly what will be saved before you commit
- **Branches for exploration** — try different color palettes, plot lines, or layouts in parallel (no merge — branches are independent creative lines)
- **Version export** — export any version as a directory, `.zip`, or `.tar.gz`
- **Text diff** — line-level diffs between versions for writers
- **Binary-aware** — large files (.psd, .blend, video) handled with streaming + progress bars, no OOM
- **Cross-platform** — Windows, macOS, Linux
- **WIP auto-save** — switching branches with pending changes auto-saves them; restore with one command
- **Version aliases** — name versions like `初稿` / `final` instead of `v1` / `v2`

## Quick Start

### Install

**Windows (installer):** Download `drift-setup-x.y.z.exe` from [Releases](../../releases) and run it — graphical installer with PATH setup and uninstaller.

**Build from source:**
```bash
go build -ldflags "-X github.com/drift/drift/internal/cli.version=0.1.0" -o dist/drift.exe ./cmd/drift/
```

Verify the install:
```bash
drift version
```

### Use

```bash
# Initialize a project (creates .drift/ in the current directory)
drift init

# Stage all files
drift add .

# Save a version
drift save -m "first draft done"

# View history
drift log --all

# Go back to an earlier version
drift restore v1

# Export a version for delivery
drift export v1 -o ./delivery
```

### Common workflows

**Writer exploring plot branches:**
```bash
drift save -m "main storyline v1"
drift branch alt-ending
drift switch alt-ending
drift save -m "alternative ending"
drift diff v1 v2 -p          # compare the two versions line by line
```

**Designer iterating on a client project:**
```bash
drift save -a -m "revision 2"   # -a auto-stages all changes, like git commit -a
drift export v2 -o ./client-v2.zip -f zip
drift restore v1 素材/封面.psd   # restore only one file from v1
```

## Commands

| Command | Description |
|---------|-------------|
| `init` | Initialize a new Drift project |
| `add` | Add files to the staging area |
| `status` | Show working tree status |
| `save` | Save staged changes as a new version (`-a` auto-stages, `--amend` edits last version, `--tag` sets a tag) |
| `log` | View commit history (`--all` across branches, `--oneline` for compact view) |
| `reflog` | View operation history (undo/redo log) |
| `restore` | Restore workspace or specific files to a version |
| `export` | Export a version as dir / zip / tar.gz |
| `diff` | Show differences between versions (`-p` for patch, `-f`/`--` for file filtering) |
| `branch` | List / create / delete / rename branches |
| `switch` | Switch branches (auto-saves WIP, `--create` to create on the fly) |
| `tag` | List / add / delete version tags |
| `undo` | Undo recent operations |
| `unstage` | Remove files from staging area (no args clears all) |
| `clean` | Remove untracked files |
| `rm` / `mv` | Delete / move tracked files |
| `config` | View and set configuration (`user.name`, `core.autocrlf`, etc.) |
| `wip` | Manage work-in-progress (`list` / `save` / `restore` / `drop`) |
| `version` | Show drift version |

Full reference: [docs/commands.md](docs/commands.md)

## Tech Stack

- **Language:** Go
- **Hashing:** SHA-256 (pure digest, not Git-compatible)
- **Storage:** Content-addressable, binary formats (DRIX / DREE / DCMT) with zlib compression
- **CLI framework:** cobra
- **Locking:** OS-level file locks (LockFileEx on Windows, flock on Unix)

## Project Structure

```
drift/
├── cmd/drift/          # CLI entry point
├── internal/
│   ├── core/           # Object model (Blob / Tree / Commit / Index), hashing, codecs, DAG walker, diff
│   ├── storage/        # Content-addressable store, atomic writes, file locking
│   ├── app/            # Business logic (save, restore, switch, diff, export, sync)
│   ├── cli/            # All cobra commands (presentation layer)
│   ├── config/         # Project-level and global JSON config read/write
│   ├── sync/           # Remote sync engine (DAG-based push/pull) and transports (WebDAV/FTP/SFTP/SMB)
│   └── worktree/       # Working tree operations (staging, WIP, clean)
├── installer/          # Inno Setup script for Windows installer
├── .github/workflows/  # CI/CD: release workflow (tag-triggered)
└── docs/               # Design docs (Chinese)
```

## Documentation

| Doc | Content |
|-----|---------|
| [Product Requirements](docs/PRD.md) | Target users, use cases, feature trade-offs |
| [Technical Design](docs/technical.md) | Architecture, data formats, cross-platform |
| [Command Reference](docs/commands.md) | Full CLI command documentation |
| [Development Progress](docs/progress.md) | Completed phases and next steps |
| [Test Plan](docs/test-plan.md) | Test cases and coverage |

## Releasing

Releases are automated via GitHub Actions. Push a version tag and the workflow will:

1. Build `drift.exe` for Windows (amd64)
2. Embed the application icon via `rsrc`
3. Compile a Windows `drift-setup-x.y.z.exe` with Inno Setup
4. Publish all artifacts to a GitHub Release with notes extracted from the changelog

```bash
git tag v1.0.0
git push origin v1.0.0
```

## Acknowledgments

Parts of this project reference or are inspired by [go-git](https://github.com/go-git/go-git), licensed under the [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0).

## License

MIT
