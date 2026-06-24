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

**Windows (from source):**
```powershell
.\install.bat
```

**macOS / Linux:**
```bash
./install.sh
```

**Build from source:**
```bash
go build -ldflags "-X github.com/drift/drift/internal/cli.version=0.1.0" -o drift ./cmd/drift/
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
| `add` | Add files to the staging area (supports globs, multiple paths) |
| `status` | Show working tree status |
| `save` | Save staged changes as a new version (`-a` auto-stages, `--amend` edits last version, `--name` sets an alias) |
| `log` | View commit history (`--all` across branches) |
| `restore` | Restore the workspace or specific files to a version |
| `export` | Export a version as dir / zip / tar.gz |
| `diff` | Show differences between versions (`-p` for patch, `-f`/`--` for file filtering) |
| `branch` | Create / list / delete / rename branches |
| `switch` | Switch branches (auto-saves WIP, `--create` to create on the fly) |
| `name` | Manage version aliases |
| `wip` / `restore-wip` | List / restore auto-saved work-in-progress |
| `rm` / `mv` | Delete / move tracked files |
| `config` | View and set configuration (`user.name`, `core.autocrlf`, etc.) |
| `history` / `undo` | View / undo recent operations |
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
│   ├── core/           # Object model (Blob / Tree / Commit / Index), hashing, codecs, diff
│   ├── storage/        # Content-addressable store, atomic writes, file locking
│   ├── cli/            # All cobra commands
│   └── config/         # JSON config read/write
├── installer/          # Inno Setup script for Windows installer
├── .github/workflows/  # CI/CD: release workflow (tag-triggered)
├── install.bat / install.sh  # Source-based install scripts
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

Releases are fully automated via GitHub Actions. Push a version tag and the workflow will:

1. Build binaries for Windows (amd64), macOS (amd64 + arm64), and Linux (amd64 + arm64)
2. Compile a Windows `setup.exe` with Inno Setup (graphical installer, PATH management, uninstaller)
3. Publish all artifacts to a GitHub Release

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License

MIT
