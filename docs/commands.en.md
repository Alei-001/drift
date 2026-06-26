# Drift — CLI Command Reference

## Design Principles

| Principle | Description |
|-----------|-------------|
| **Simplicity** | Fewer commands than Git |
| **Intuitiveness** | Command names describe their function (save / log / export / restore) |
| **Non-technical friendly** | Designed for creative workers with minimal learning curve |

## Command List

| Command | Description | Status |
|---------|-------------|--------|
| `init` | Initialize a project | ✅ |
| `add` | Add files to staging area | ✅ |
| `status` | Show working tree status | ✅ |
| `unstage` | Clear or remove files from staging | ✅ |
| `save` | Save a version | ✅ |
| `log` | View commit history (`--all` for all branches) | ✅ |
| `export` | Export a version | ✅ |
| `restore` | Restore working tree | ✅ |
| `diff` | Show differences | ✅ |
| `branch` | Manage branches | ✅ |
| `switch` | Switch branches | ✅ |
| `config` | Configuration management | ✅ |
| `rm` | Remove files | ✅ |
| `mv` | Move / rename files | ✅ |
| `tag` | Version tags | ✅ |
| `reflog` | Operation history | ✅ |
| `undo` | Undo operations | ✅ |
| `wip` | Work-in-progress management | ✅ |
| `clean` | Remove untracked files | ✅ |
| `version` | Show version number | ✅ |

---

## Initialization

### `drift init` ✅

Initialize a new Drift project.

```bash
drift init
```

**Behavior:**
- Creates `.drift/` directory in the current working directory
- Initializes storage structure and default configuration
- Creates `main` branch and sets HEAD
- If global `user.name` and `user.email` are already configured, skips the prompt and uses them
- Otherwise, prompts interactively for name and email (with email format validation)
- User identity is saved to **global config** (`~/.drift/global.json`), shared across all projects
- Shows next-step guidance

---

## Staging Commands

### `drift add` ✅

Add files to the staging area.

```bash
drift add <path> [<path>...]    # multiple paths
drift add .                     # all files
drift add chapters/             # entire directory
drift add *.txt                 # glob patterns
drift add assets/ chapters/ch1.txt  # mix of directories and files
```

**Behavior:**
- Supports multiple path arguments in one invocation
- Supports glob patterns (`*`, `?`, `[...]`)
- Ignores duplicate adds of unchanged content
- Computes SHA-256 hashes, stores blobs under `objects/blobs/`
- Updates the staging index

### `drift status` ✅

Show working tree status.

```bash
drift status                  # human-readable (default)
drift status --porcelain      # machine-readable
```

**Output (default):**

```
On branch main, version v2

Staged changes:
  A chapters/ch1.txt
  M chapters/ch2.txt

Unstaged changes:
  D assets/old.png

Untracked files:
  notes.txt
```

**Output (--porcelain):**

```
A chapters/ch1.txt
M chapters/ch2.txt
 D assets/old.png
?? notes.txt
```

**Status codes:**
- `A` — Added
- `M` — Modified
- `D` — Deleted
- `?` — Untracked

**Porcelain format:** Each line is `XY <path>` where X is the staging status and Y is the working tree status.

### `drift unstage` ✅

Remove files from the staging area.

```bash
drift unstage                 # clear entire staging area
drift unstage <path>          # remove only the specified file
```

**Behavior:**
- With no arguments, clears the entire staging area
- With a path argument, removes only that file
- Does not affect working tree files

---

## Version Commands

### `drift save` ✅

Save staged changes as a new version.

```bash
drift save                        # no message
drift save -m "message"           # with message
drift save --tag v1               # save and set tag
drift save -m "first draft" --tag v1  # message + tag
drift save --amend                # amend the last version
drift save --amend -m "new message"   # amend with new message
drift save -a -m "message"        # auto-stage all changes, then save
drift save --all                  # equivalent to drift add . + drift save
```

`-m` / `--message` is optional. `--tag` sets a tag on the new version. `-a` / `--all` auto-stages all working tree changes before saving.

**Behavior:**
- Builds a Tree object from the staging area
- Creates a Commit object; the version ID is the first 8 hex characters of the commit hash
- Updates the current branch ref and clears the staging area
- Rejects save if no files have changed since the last version
- Lists actually changed files after saving
- `--tag` creates a version tag after successful save
- `--all` stages all changes (adds, modifications, deletions of tracked files) before saving

**--amend behavior:**
- Replaces the most recent commit, generating a new version ID
- Updates the Tree and Message
- Used to correct the last saved version

### `drift log` ✅

View commit history.

```bash
drift log                      # current branch, all history
drift log <branch>             # specific branch history
drift log --all                # all branches (deduplicated by hash, newest first)
drift log --oneline            # compact one-line format
drift log -n 5                 # last 5 commits
drift log main -n 10           # last 10 on main branch
drift log --all --oneline      # all branches, compact
```

**Flags:**

| Flag | Description |
|------|-------------|
| `<branch>` | Optional — show history for a specific branch |
| `--all` | Show commits from all branches (deduplicated by hash) |
| `--oneline` | Compact one-line format |
| `-n` / `--number` | Limit to N commits (0 = all) |
| `--porcelain` | Machine-readable format |

**Output (full mode, single branch):**

```
  commit abc123def456...
  Version: a1b2c3d4
  Tags:    v1
  Branch:  main
  Date:    2024-06-15 10:30:00
  Author:  Alice <alice@example.com>

      Finished first four chapters

commit def456abc123...
Version: e5f6a7b8
Branch:  main
Date:    2024-06-15 09:00:00

      Changed color scheme
```

**Output (--oneline):**

```
a1b2c3d4 v1 [main] Finished first four chapters
e5f6a7b8    [main] Changed color scheme
c9d0e1f2    [main] Initial commit
```

> **Note:** `drift log` without arguments shows the current branch history. `drift log --all` shows all branches deduplicated.

### `drift export` ✅

Export a specific version to the filesystem.

```bash
drift export <version> -o <output> [-f <format>] [<path>...]

# Basic usage
drift export a1b2c3d4 -o ./delivery          # export to directory (default)
drift export a1b2c3d4 -o ./delivery.zip -f zip   # export as zip
drift export a1b2c3d4 -o ./delivery.tar.gz -f tar  # export as tar.gz
drift export main -o ./main-snapshot          # export branch tip
drift export v1 -o ./draft                    # export using tag

# Path filtering
drift export a1b2c3d4 -o ./subset chapters/       # only chapters/ directory
drift export a1b2c3d4 -o ./subset chapters/ notes/  # multiple directories
drift export a1b2c3d4 -o ./single chapters/ch1.txt  # single file
```

**Arguments:**
- `<version>` — version ID (e.g. `a1b2c3d4`, supports prefix), branch name (e.g. `main`), or tag (e.g. `v1`)
- `-o` / `--output` — **required**, output path
- `-f` / `--format` — optional, `dir` (default) / `zip` / `tar`
- `<path>...` — optional, files or directories to export. If omitted, exports the entire version.

### `drift restore` ✅

Restore the working tree to a specified version.

```bash
drift restore <version>                    # restore entire working tree
drift restore <version> --force            # force restore (discard staged changes)
drift restore main                         # restore to branch tip
drift restore a1b2c3d4 chapters/ch1.txt    # restore only specified file
drift restore a1b2c3d4 chapters/ch1.txt notes/  # restore specific files / directories
drift restore v1                           # restore using tag
```

**Arguments:**
- `<version>` — version ID (prefix supported), branch name, or tag
- `<path>...` — optional, files or directories to restore. If omitted, restores the entire working tree.

**Behavior:**
- Restores working tree files to the target version
- Does **not** change branch references (only changes working tree content)
- Requires `--force` if staging area differs from current version
- Untracked files are left untouched
- When paths are specified, only matching files are restored

> **Note:** `restore` only changes working tree content; branch ref stays unchanged. Use path arguments to roll back individual files.

---

## File Management

### `drift rm` ✅

Remove files and unstage them.

```bash
drift rm <path> [<path>...]       # remove one or more files
drift rm --cached <path>          # unstage only, keep on disk
drift rm -r <directory>           # recursively remove directory
drift rm *.tmp                    # glob patterns
drift rm -f *.tmp                 # skip confirmation
drift rm --dry-run *.tmp          # preview only
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--cached` | Unstage only, keep disk file |
| `-r` / `--recursive` | Recursively remove directory (required) |
| `-f` / `--force` | Skip confirmation prompt |
| `--dry-run` | Preview mode — list what would be removed |

**Behavior:**
- Only removes files tracked by Drift; untracked files are silently skipped
- By default removes both the working tree file and the staging entry
- Interactive confirmation before deletion unless `-f` is used
- `--cached` keeps the disk file, removing only the staging entry
- Automatically cleans up empty parent directories

**Examples:**

```bash
drift rm chapters/old.txt
drift rm -r old-assets/
drift rm *.tmp *.bak
drift rm --cached large-file.psd
drift rm chapters/old.txt
drift status              # shows D (deleted)
drift save -m "remove old draft"
```

### `drift mv` ✅

Move or rename tracked files.

```bash
drift mv <source> <target>              # rename a file
drift mv <file> <directory>             # move into directory
drift mv <file1> <file2> <directory>    # move multiple files into directory
```

**Behavior:**
- Operates on tracked files (staged or committed)
- Updates both the working tree and staging area
- When target is an existing directory, moves source into it
- Automatically cleans up empty source directories

**Examples:**

```bash
drift mv chapters/ch1.txt chapters/prologue.txt
drift mv cover.png assets/
drift mv cover.png illustration.png assets/
drift mv old-chapters/ new-chapters/
drift mv chapters/ch1.txt chapters/prologue.txt
drift status
drift save -m "rename chapter 1 to prologue"
```

---

## Branch Commands

### `drift branch` ✅

Create, list, delete, or rename branches.

```bash
drift branch <name>         # create branch from current version
drift branch list           # list all branches
drift branch -d <name>      # delete branch
drift branch -m <new> <old> # rename branch
```

**Output:**

```
* main
  experiment
  alt-ending
```

**Constraints:**
- Cannot delete HEAD
- Cannot delete the currently checked-out branch
- Cannot rename to an existing branch name
- If on the renamed branch, HEAD auto-updates

**Operation log:** Deletes and renames are recorded in the operation history; can be undone with `drift undo`.

### `drift switch` ✅

Switch to a different branch.

```bash
drift switch <name>
drift switch <name> --force     # discard unsaved changes
drift switch <name> --create    # create branch if it doesn't exist
drift switch <name> -c          # shorthand for --create
```

**Behavior:**
- Switches the working tree to the target branch's latest version
- Auto-saves WIP (work-in-progress) when there are unstaged changes
- `--force` discards unsaved changes
- `--create` / `-c` creates the branch if it doesn't exist
- After switching, use `drift wip restore` to recover auto-saved work

> **Design principle:** Branches are independent creative lines. No merge. Writers can explore different plot lines; designers can try different color palettes.

---

## Diff Command

### `drift diff` ✅

Show differences between versions.

**Summary (default):**

```bash
drift diff                         # working tree vs current branch tip
drift diff a1b2c3d4                # working tree vs specific version
drift diff a1b2c3d4 e5f6a7b8       # two versions
drift diff v1 v2                   # two tags (works across branches)
drift diff v1 main                 # tag vs branch tip
drift diff main feature            # main tip vs feature tip
```

**Summary output:**

```
Changed 2 file(s):
  M chapters/ch1.txt
  A notes/new.txt
```

**Detailed diff:**

```bash
drift diff -p                      # working tree vs current branch (detailed)
drift diff a1b2c3d4 e5f6a7b8 -p    # two versions (detailed)
drift diff v1 v2 -p                # two tags (detailed)
```

**Detailed output (text files):**

```
--- chapters/ch1.txt
+++ chapters/ch1.txt
 Chapter 1 Begins

-This is the start of a story.
+This is the start of an adventure.
+
+The protagonist is a young traveler.

 The weather was clear.
```

**Binary files:**

```
Binary files differ: assets/cover.psd
```

**File filtering:**

```bash
drift diff a1b2c3d4 e5f6a7b8 --file chapters/ch1.txt
drift diff a1b2c3d4 e5f6a7b8 -f chapters/ch1.txt
drift diff a1b2c3d4 e5f6a7b8 -- chapters/ch1.txt
drift diff v1 v2 -f chapters/
drift diff a1b2c3d4 e5f6a7b8 --file chapters/ --file notes/
```

**Flags:**

| Flag | Description |
|------|-------------|
| `-p` / `--patch` | Show detailed line-level diffs |
| `--file <path>` | Filter to specific files (repeatable) |
| `-f <path>` | Shorthand for `--file` |
| `-- <path>...` | Paths after `--` separator, no flag needed |
| `-o` / `--output <file>` | Write diff to file |

**Version identifiers:**

| Format | Example | Description |
|--------|---------|-------------|
| Version ID | `a1b2c3d4` | First 8 chars of commit hash (prefix supported) |
| Branch name | `main` | Tip of that branch |
| Branch/version | `main/a1b2c3d4` | Specific commit on a branch |
| Tag | `v1` | Tag set via `drift tag` |

> **Note:** Drift does not merge. Cross-branch diffs are for viewing, not merging.

---

## Tag Commands

### `drift tag` ✅

Set friendly labels on versions.

```bash
drift tag <version> <tag-name>    # set tag on version
drift tag                         # list all tags (bare command)
drift tag list                    # list all tags (explicit)
drift tag --delete <tag-name>     # delete a tag
```

**Examples:**

```bash
drift tag a1b2c3d4 v1
drift tag e5f6a7b8 final
drift tag a1b2 v1                # version ID prefix OK
drift tag                         # list all tags
drift tag --delete v1
```

**Behavior:**
- Tags are global and unique (stored as `refs/tags/<name>.ref`)
- Usable in all commands that accept a version (`diff`, `export`, `restore`, etc.)
- A commit can have multiple tags (shown comma-separated in `drift log`)
- A tag cannot be reassigned to a different commit; delete it first

---

## Operation History

### `drift reflog` ✅

View operation history.

```bash
drift reflog                   # last 20 entries (default)
drift reflog -n 10             # last 10
drift reflog -n 0              # all entries
drift reflog -v                # verbose (show ref changes)
drift reflog --porcelain       # machine-readable
```

**Flags:**

| Flag | Description |
|------|-------------|
| `-n` / `--number` | Number of entries (default 20; 0 = all) |
| `-v` / `--verbose` | Show ref change details |
| `--porcelain` | Machine-readable format |

**Output (default, tabular with header):**

```
DATE                OP      DESCRIPTION
2026-06-26 20:08:18  save    save 5db1a613 (mv test) on sec
2026-06-26 19:45:05  switch  switch to sec
```

**Output (verbose, `-v`):**

```
DATE                OP      DESCRIPTION
2026-06-26 20:22:32  tag-add  tag 5db1a613 as v1.0.3
  tags/v1.0.3: -       → 5db1a613
2026-06-26 20:08:18  save    save 5db1a613 (mv test) on sec
  sec:           012f9f08 → 5db1a613
2026-06-26 19:45:05  switch  switch to sec
  HEAD:          acd     → sec
```

**Recorded operation types:**
- `save` — version saved
- `switch` — branch switch
- `branch-delete` — branch deleted
- `branch-rename` — branch renamed
- `restore` — working tree restored
- `tag-add` / `tag-delete` — tag added or deleted

### `drift undo` ✅

Undo recent operations.

```bash
drift undo                    # undo last operation
drift undo -n 3               # undo last 3 operations
```

**Flags:**

| Flag | Description |
|------|-------------|
| `-n` / `--number` | Number of operations to undo (default 1) |

**Behavior:**
- Restores branch references to their pre-operation state
- Undoing a `save` removes that commit and restores the branch pointer
- Undoing `branch -d` restores the deleted branch
- Undoing `branch -m` restores the original name
- Batch undo with `-n` rolls back from newest to oldest

---

## Work-in-Progress

### `drift wip` ✅

Manage unsaved work-in-progress. When switching branches with unsaved changes, WIP is auto-saved.

```bash
drift wip list [<branch>]        # list WIP (no args = all branches)
drift wip save                   # manually save current work as WIP
drift wip restore [<branch>]     # restore WIP to working tree
drift wip drop [<branch>]        # discard WIP (requires confirmation)
```

**Examples:**

```bash
drift wip list
drift wip list main
drift wip save
drift wip restore
drift wip restore feature
drift wip drop
```

**Behavior:**
- WIP is auto-saved on branch switch when there are unsaved changes
- Stored as `.drift/wip/<branch>.json`
- Restoring WIP deletes the WIP file afterwards
- `drop` requires a second confirmation

---

## Clean Command

### `drift clean` ✅

Remove untracked files from the working tree.

```bash
drift clean                   # remove untracked files
drift clean -n                # dry-run: show what would be removed
drift clean -f                # force: skip confirmation
```

---

## Configuration

### `drift config` ✅

View or set configuration. Supports project-level (`.drift/config.json`) and global (`~/.drift/global.json`).

```bash
drift config                       # list project config (bare command)
drift config list                  # list project config (explicit)
drift config --global              # list global config
drift config <key>                 # get a value
drift config <key> <value>         # set a project value
drift config --global <key> <value># set a global value
drift config --unset <key>         # unset a project value
drift config --global --unset <key># unset a global value
```

**Configuration hierarchy:**

| Level | File | Description |
|-------|------|-------------|
| Global | `~/.drift/global.json` | Shared across all projects — default user identity and remote sync config |
| Project | `.drift/config.json` | Per-project overrides |

**Supported keys:**

| Key | Scope | Description | Default |
|-----|-------|-------------|---------|
| `user.name` | global / project | Author name | (empty) |
| `user.email` | global / project | Author email | (empty) |
| `core.default_branch` | project | Default branch name | `main` |
| `core.autocrlf` | project | CRLF normalization strategy | `""` (disabled) |
| `sync.enabled` | project | Enable auto-sync | `false` |
| `remote.protocol` | global | Remote protocol (webdav/ftp/sftp/smb) | (empty) |
| `remote.host` | global | Remote host address | (empty) |
| `remote.port` | global | Remote port | (empty) |
| `remote.path` | global | Remote base path | (empty) |
| `remote.username` | global | Remote username | (empty) |
| `remote.tls` | global | Enable TLS | `false` |
| `remote.insecure_skip_verify` | global | Skip TLS certificate verification | `false` |
| `remote.share` | global | SMB share name (SMB only) | (empty) |
| `remote.key_path` | global | SSH private key path (SFTP only) | (empty) |

**Output (section-grouped):**

```bash
$ drift config
[core]
  autocrlf       =
  default_branch = main
[sync]
  enabled        = false
[user]
  name           =
  email          =
```

**Examples:**

```bash
drift config --global user.name "Alice"
drift config --global user.email "alice@example.com"
drift config user.name "project-specific"
drift config user.name                     # read current value
drift config --global                      # list all global config
drift config --unset user.email
drift config core.autocrlf true
drift config sync.enabled true
```

### `drift config remote` ✅

Configure a remote sync endpoint. Supports four protocols: WebDAV, FTP/FTPS, SFTP, and SMB/CIFS.

```bash
# WebDAV
drift config remote --protocol webdav --host cloud.example.com --path /dav \
  --tls --user alice --pass secret

# FTP / FTPS
drift config remote --protocol ftp --host nas.local --path /backups \
  --user alice --pass secret

# SFTP (password or key auth)
drift config remote --protocol sftp --host nas.local --path /backups --user alice
drift config remote --protocol sftp --host nas.local --user alice --key-path ~/.ssh/id_rsa

# SMB / CIFS
drift config remote --protocol smb --host nas.local --share photos --user alice

# URL shorthand
drift config remote https://cloud.example.com/dav --user alice --pass secret

# Manage remote config
drift config remote --show
drift config remote --unset
```

**Flags:**

| Flag | Description |
|------|-------------|
| `--protocol` | Protocol: `webdav` / `ftp` / `sftp` / `smb` |
| `--host` | Remote hostname or IP |
| `--port` | Port (0 = protocol default) |
| `--path` | Remote base path |
| `--user` | Username (prompted if omitted) |
| `--pass` | Password (prompted if omitted) |
| `--tls` | Enable TLS (FTPS, HTTPS) |
| `--insecure` | Skip TLS certificate verification |
| `--share` | SMB share name (SMB only) |
| `--key-path` | SFTP private key path |
| `--show` | Show current remote config |
| `--unset` | Clear remote config |

**Default ports:**

| Protocol | Default Port | TLS Support |
|----------|-------------|-------------|
| `webdav` | 80 (http) / 443 (https) | `--tls` |
| `ftp` | 21 | `--tls` (FTPS) |
| `sftp` | 22 | Built-in SSH encryption |
| `smb` | 445 | — |

> **Note:** Sync and clone commands are temporarily disabled in v1.0.0 pending integration testing. Remote configuration can be set up, but `drift sync` and `drift clone` are not yet available.

---

## Help & Version

```bash
drift --help
drift <command> --help
```

### `drift version` ✅

Show the drift version.

```bash
drift version
```

```
drift 1.0.0
```

> The version is injected at build time via ldflags. Development builds (`go run` / `go test`) show `dev`.

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
| `not a drift project (run 'drift init')` | Not initialized | Run `drift init` |
| `file not found` / `path not found` | Wrong path | Check file path |
| `nothing to save (use 'drift add' first)` | Empty staging | Run `drift add` |
| `nothing changed since last version` | No file changes | Edit files, then `drift add` |
| `version not found` | Wrong version ID | Run `drift log --all` |
| `staging area has pending changes` | Staging not empty | Run `drift unstage` or use `--force` |
| `branch not found` | Wrong branch name | Run `drift branch list` |
| `branch already exists` | Duplicate name | Use a different name or `branch -m` |
| `cannot delete the current branch` | Deleting active branch | `drift switch` to another branch first |
| `could not acquire lock` | Another drift process running | Wait or check `.drift/lock` |
| `pathspec did not match any tracked files` | Not a tracked file | `drift add` first |
| `no version to amend` | No commits yet | `drift save` first |
