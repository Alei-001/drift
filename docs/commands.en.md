# Drift — CLI Command Reference

## Design Principles

| Principle | Description |
|-----------|-------------|
| **Simplicity** | Fewer commands than Git |
| **Intuitiveness** | Command names describe their function (save / history / back / export) |
| **Non-technical friendly** | Designed for creative workers with minimal learning curve |

## Command List

| Command | Description |
|---------|-------------|
| `start` | Initialize a new project |
| `save` | Save all changes as a new version |
| `history` | View version history |
| `back` | Restore working tree to a version |
| `diff` | Show differences |
| `export` | Export a version |
| `undo` | Undo operations |
| `branch` | Manage branches (create/list/switch/remove/rename) |
| `tag` | Manage version tags (add/list/remove) |
| `move` | Move / rename files |
| `remove` | Remove files |
| `ignore` | Add patterns to .driftignore |
| `status` | Show working tree state |
| `remote` | Remote backup configuration (setup/show/remove) |
| `backup` | Remote backup operations (on/off/now/status/log) |
| `clone` | Clone project from remote |
| `whoami` | Show or set your identity |
| `version` | Show version number |

---

## Project Setup

### `drift start`

Initialize a new Drift project in the current directory.

```bash
drift start
drift start --name "Alice" --email "alice@mail.com"
```

**Behavior:**
- Creates `.drift/` directory with storage structure
- Creates `main` branch
- Prompts interactively for identity if not provided on command line
- Identity saved to global config (`~/.drift/global.json`)
- Shows next-step guidance

---

## Version Commands

### `drift save`

Save all working tree changes as a new version. **No prior `add` required.** Auto-detects modified, added (respecting `.driftignore`), and deleted files.

```bash
drift save                    # no message
drift save -m "Chapter 3 done"
drift save --tag v1           # save + tag in one step
drift save -m "final" --tag v1
```

**Behavior:**
- Auto-scans working directory, detects all file changes
- Respects `.driftignore` exclusion patterns
- Line endings auto-normalized (CRLF→LF) for text files; binary files preserved as-is
- Creates a commit; version ID is first 8 hex chars of commit hash
- Updates the current branch ref
- Refuses save if nothing changed since last version
- `--tag` creates a tag after successful save (tag conflict is a non-fatal warning)

**Output:**
```
Saved version a1b2c3d4: Chapter 3 done
  3 file(s) changed:
    M  chapters/ch3.md
    A  notes/characters.md
    D  assets/unused.png
```

---

### `drift history`

View version history.

```bash
drift history                    # current branch, full format
drift history main               # specific branch
drift history --all              # all branches (deduplicated, newest first)
drift history --brief            # compact one-line format
drift history --all --brief      # all branches, compact
drift history -n 5               # last 5 commits
drift history --porcelain        # machine-readable
```

**Arguments:**

| Argument | Description |
|----------|-------------|
| `<branch>` | Optional — specific branch to view |
| `--all` | Show commits from all branches |
| `--brief` | Compact one-line format |
| `-n` / `--number` | Limit count (0 = all) |
| `--porcelain` | Machine-readable |

**Output (full mode):**
```
Version:  a1b2c3d4
Tags:     v1, final
Branch:   main
Date:     2026-06-27 15:30:00
Author:   Alice <alice@mail.com>

    Chapter 3 draft
```

**Output (--brief):**
```
VERSION     BRANCH    MESSAGE                         TAG
a1b2c3d4    main      Chapter 3 draft                   v1, final
e5f6a7b8    main      Outline
```

---

### `drift back`

Restore the working tree to a specified version.

```bash
drift back                        # restore to latest version (discard unsaved changes)
drift back a1b2c3d4              # restore to specific version
drift back v1                    # restore using tag
drift back main                  # restore to branch tip
drift back a1b2c3d4 chapters/    # restore only specified paths
drift back --force               # force (skip unsaved changes warning)
```

**Behavior:**
- Restores working tree files to the target version
- Does **not** change branch references
- Refuses if there are unsaved changes (use `--force` to discard)
- When paths are specified, only matching files are restored
- Untracked files are left untouched

---

### `drift diff`

Show file differences between versions.

```bash
drift diff                         # working tree vs latest version
drift diff a1b2c3d4                # working tree vs specific version
drift diff a1b2c3d4 e5f6a7b8       # two versions
drift diff v1 v2                   # two tags
drift diff main feature            # branch tips
```

**Arguments:** `-p` for detailed diff, `--file`/`-f` for path filtering, `-o` for file output.

---

### `drift export`

Export a version to the filesystem.

```bash
drift export a1b2c3d4 --to ./delivery
drift export a1b2c3d4 --to ./delivery.zip --format zip
drift export v1 --to ./draft chapters/
```

**Arguments:** `<version>`, `-o` / `--output` (required), `-f` / `--format` (dir/zip/tar), optional path filters.

---

### `drift undo`

Undo recent operations.

```bash
drift undo                    # undo last operation
drift undo -n 3               # undo last 3 operations
```

**Undoable operations:** `save` (removes commit), `branch remove` (restores branch), `branch rename` (restores original name), `tag add` (removes tag), `tag remove` (restores tag).

---

## Branch Commands

### `drift branch`

All branch operations are unified under the `branch` command.

```bash
drift branch create <name>     # create branch from current version and switch
drift branch list              # list all branches
drift branch switch <name>     # switch to a branch
drift branch remove <name>     # delete a branch
drift branch rename <old> <new> # rename a branch
```

**Output:**
```
* main
  dark-ending
  plan-b
```

**Create:** Creates a new branch from the current commit and switches to it. Unsaved changes are auto-stashed.

**Constraints:** Cannot delete or rename HEAD. Cannot delete the currently checked-out branch.

---

## Tag Commands

### `drift tag`

Tags give versions human-readable names. Unicode (Chinese, emoji) is fully supported.

```bash
drift tag add <version> <name>        # tag a version
drift tag list                        # list all tags
drift tag remove <name>               # remove a tag
```

**Examples:**
```bash
drift tag add a1b2c3d4 v1
drift tag add a1b2c3d4 "final draft"  # Unicode OK
drift tag list
```

**Output:**
```
v1          → a1b2c3d4  Chapter 3 draft
final draft → f3c8a1b2  Final version
```

---

## File Management

### `drift move`

Move or rename tracked files.

```bash
drift move <source> <target>
drift move <file> <directory>
drift move <file1> <file2> <directory>
```

---

### `drift remove`

Remove tracked files.

```bash
drift remove <path> [<path>...]
drift remove --cached <path>          # un-track only, keep on disk
drift remove -r <directory>
drift remove --force
drift remove --dry-run
```

---

### `drift ignore`

Add a pattern to `.driftignore`. The matched files will no longer be auto-tracked by `drift save`.

```bash
drift ignore "*.psd"
drift ignore "build/"
```

**Behavior:** Appends the pattern to `.driftignore` at the project root (gitignore-compatible syntax). Does not add duplicate patterns.

---

## Working Tree Status

### `drift status`

Show the current working tree state.

```bash
drift status                 # human-readable
drift status --porcelain     # machine-readable
```

**Output:**
```
On branch main, version a1b2c3d4

Unsaved changes:
  M chapters/ch3.md
  D assets/unused.png

New files (not yet saved):
  notes/new-note.txt
```

**Status codes:** `A` = Added, `M` = Modified, `D` = Deleted, `?` = Untracked.

---

## Remote Backup Configuration

### `drift remote`

Configure a remote backup destination. Supports WebDAV, FTP/FTPS, SFTP, and SMB/CIFS.

```bash
drift remote setup          # interactive configuration
drift remote show           # show current remote config
drift remote remove         # remove remote config
```

**`remote setup` interactive flow:**
```
? Protocol (webdav/ftp/sftp/smb): webdav
? Host: cloud.example.com
? Port (0=default): 443
? Path: /dav/novels
? Username: alice
? Password: ****
? TLS? (y/N): y
Remote saved: webdav://alice@cloud.example.com/dav/novels
```

**Password security:** Encrypted with AES-256-GCM and stored in `~/.drift/global.json`; the encryption key is in `~/.drift/.key` (0600 permissions). Also supports the `DRIFT_REMOTE_PASSWORD` environment variable.

**Default ports:**

| Protocol | Default Port | TLS Support |
|----------|-------------|-------------|
| `webdav` | 80 / 443 | `--tls` |
| `ftp` | 21 | `--tls` (FTPS) |
| `sftp` | 22 | Built-in SSH |
| `smb` | 445 | — |

---

## Remote Backup Operations

### `drift backup`

Enable, disable, or trigger remote backup. Requires `drift remote setup` first.

```bash
drift backup on               # enable auto-backup (triggered by each `save`)
drift backup off              # disable auto-backup
drift backup now              # manual backup
drift backup status           # show backup status
drift backup log              # show backup history
```

**`backup status` output:**
```
Auto-backup: ON
Remote:      webdav://cloud.example.com/dav/novels
Last backup: 2026-06-27 15:30:00
```

**Conflict strategy:** Local version wins ("last save wins"). Drift is a personal backup tool, not a multi-device collaboration system.

---

### `drift clone`

Clone a project from remote backup.

```bash
drift clone my-novel                  # clone to ./my-novel
drift clone my-novel --to ~/work/     # clone to specified directory
```

**Prerequisite:** Remote must be configured via `drift remote setup`.

---

## Identity

### `drift whoami`

Show or set the author identity. Used in version records. Saved to global config (all projects) or local config (per-project override).

```bash
drift whoami                              # show current identity
drift whoami set "Alice" "alice@mail.com"  # set global identity
drift whoami set "Bob" "bob@mail.com" --local  # set project-level identity
```

---

## Help

```bash
drift --help
drift <command> --help
```

### `drift version`

Show the drift version number.

```
drift dev
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |

---

## Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `not a drift repository (run 'drift start')` | Not initialized | Run `drift start` |
| `file not found` / `path not found` | Wrong path | Check file path |
| `nothing changed since last version` | No file changes | Edit files, then `drift save` |
| `version not found` | Wrong version ID | Run `drift history --all` |
| `branch not found` | Wrong branch name | Run `drift branch list` |
| `branch already exists` | Duplicate name | Use a different name |
| `cannot delete the currently checked-out branch` | Cannot delete active branch | `drift branch switch` first |
| `could not acquire lock` | Another drift process running | Wait or check `.drift/lock` |
| `not a drift project` | Not initialized | `drift start` |
| `unsafe symlink` | Symlink points outside repository | Use in-repo paths |
