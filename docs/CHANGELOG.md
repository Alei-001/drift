# Changelog

## v1.1.0 (2026-06-27)

### New Features

- `drift gc` removes unreachable objects to reclaim disk space. Objects
  reachable from any branch, tag, HEAD, or reflog entry are preserved.
- `drift upgrade [<version>]` self-upgrades drift by downloading the
  specified (or latest) binary from GitHub Releases. `--check` previews
  without downloading.
- `gc.auto` config key controls automatic GC threshold (default 1000 loose
  objects, set to 0 to disable). GC runs after `drift save` and
  `drift branch delete`.
- `gc.reflogExpire` config key (default 90 days) limits reflog-based object
  retention: entries older than the cutoff are ignored during GC, allowing
  objects from ancient amended commits to be reclaimed.

### Bug Fixes

- `drift diff` default summary now uses A / D (not + / -) for added and
  deleted files, matching `drift status` labels.
- `drift diff --patch` now correctly shows content changes for empty files
  and new files (previously produced empty diff bodies).

### Improvements

- Diff performance: LazyDiffTrees integrated across all diff paths, index
  mtime fast-path avoids re-reading unchanged files, and the tree builder
  reuses subtrees from the base tree when computing new commits.

---

## v1.0.0 (2026-06-26)

### New Features

- **Object-level sync engine** — push/pull/clone now operate on the commit
  DAG instead of scanning every file on disk. Incremental bound is a tracking
  ref; no manifest file needed. Supports WebDAV, FTP, SFTP, and SMB.
  *(Sync and clone commands are hidden in this release pending integration
  testing.)*
- `drift config` (bare), `drift tag` (bare), and `drift branch` (bare) all
  list content by default. The `--list` flag has been removed.
- `drift config` output is now section-grouped (`[core]`, `[sync]`, `[remote]`,
  `[user]`) and displays all keys including their defaults.
- `drift init` skips the interactive prompt when global `user.name` and
  `user.email` are already configured.
- Email format validation on `drift init` — re-prompts on invalid input.
- `drift log` oneline mode shows a table with VERSION / MESSAGE / TAG columns.
  Multiple tags on a commit are comma-joined.
- `drift branch` list is now sorted by last commit time (was alphabetical).

### Improvements

- `drift reflog` redesigned: header row, wider OP column, description truncated
  by width, parenthesised-message-first truncation for readability.
- `drift config remote` replaces `drift sync remote`. Use `drift config remote
  --protocol <type> ...` to set up a remote. `drift sync` now only manages
  enable / disable / status / now.
- `drift wip` subcommands: `list`, `save`, `restore`, `drop`.
- `drift unstage` with no arguments now clears the entire staging area.

### Bug Fixes

- Pull now restores the working directory after downloading objects.
- Clone now saves an index file so `drift status` works immediately.
- Branch list check now correctly handles explicit `drift branch list`
  before create.
- `drift diff <tag1> <tag2>` now works correctly with tag-based comparison.
