# Changelog

## v1.1.1 (2026-06-27)

### Bug Fixes

- **Save no longer loses subdirectory files.** A `GetTree` bug caused unchanged
  subdirectory trees to return an empty hash, which silently corrupted new
  commits — `drift save` would drop all entries in subdirectories. Fixed by
  correctly setting the Tree hash after unmarshal.
- **Subtree reuse optimization restored.** The tree builder's
  `BuildFromIndexWithBase` fast-path compared newly built (unsorted) entries
  against stored (sorted) entries, which never matched and silently fell back
  to rebuilding every subtree from scratch on every save.
- **`--no-color` flag now works.** Variable shadowing in `BuildRootCmd` caused
  the persistent flag to bind to a local variable, leaving the package-level
  variable used by `useColor()` permanently false.
- **Windows autocrlf: status no longer reports false modifications.** 
  `HasModifications` now uses LF-normalized hash comparison when `autocrlf=true`
  on Windows, matching the blob storage format.
- **Symlinks in working tree now correctly reported.** `drift status` 
  previously always flagged symlinks as Modified because `CalculateHashFromFile` 
  followed the symlink instead of comparing the target path string.
- **Path traversal hardened.** `ExpandAddPaths`, `NormalizePathFilters`, and
  `Clone` now validate paths with `ValidateTreePath` before any filesystem
  access, preventing directory escape attacks.
- **SFTP host key verification.** SFTP connections now verify host keys via
  `~/.ssh/known_hosts` by default. Use `insecure_skip_verify: true` to bypass.
- **FTP sync directory creation fixed.** `mkdirAll` incorrectly duplicated the
  base path prefix, causing objects to be stored at wrong remote paths.
- **Push panic on first push fixed.** `trackingHash[:8]` panicked when the
  tracking ref didn't exist yet (empty string). All hash slicing in the sync
  engine is now guarded by `shortHash()`.
- **Commit hash integrity.** `NewCommit` now validates that message, author
  name, and email contain no NUL bytes (which would corrupt the hash). Returns
  `error` alongside the commit.
- **Mode changes now visible in diff.** `DiffTrees` and `LazyDiffTrees` now
  compare file modes in addition to hashes, so `chmod +x` changes appear in
  `drift diff`.
- **Commit marshal validation.** `Commit.Marshal` now rejects empty hashes
  before encoding, preventing corrupt commit files.
- **Error handling hardened across the codebase.** `BranchCreate`, `Switch`,
  `Save`, `Restore`, `Push`, `Pull`, `TagAdd`, `Move`, `ResolveCommit`, and
  `ListRefs` no longer silently discard I/O errors from `GetRef`, `GetTree`,
  `GetCommit`, `currentCommit`, and `filepath` operations. The `ErrObjectNotFound`
  case is explicitly distinguished from real failures.
- **Stale lock detection.** The lock polling loop now checks whether the
  recorded PID is still alive and breaks stale locks immediately instead of
  waiting for the full 5-second timeout.
- **`Chdir` atomicity.** `App.Chdir` no longer mutates `a.store` before
  validating config, preventing the App from entering a half-switched state
  on config load failure.
- **SyncEnable nil config guard.** `SyncEnable` and `SyncDisable` now check
  for nil `a.config` before accessing sync settings.
- **`StageWorktreeChanges` handles deletions.** WIP save now removes index
  entries for files deleted from the working tree, preventing stale WIP
  snapshots.
- **`DeleteRef` updates HEAD.** Deleting the currently checked-out branch now
  clears HEAD inside the lock, preventing a dangling pointer.
- **`lock()` returns nil unlocker on error.** The previous no-op function
  returned on lock failure has been replaced with `nil` to make misuse
  visible.
- **Two sentinel errors unified.** `ErrCorruptedObject` (decompression) and
  `ErrObjectCorrupted` (hash mismatch) have been consolidated into a single
  `ErrObjectCorrupted` sentinel, so callers checking with `errors.Is` cover
  both cases.

### Improvements

- **Colorized CLI output.** All 21 commands now use consistent ANSI colors:
  green for success/added, yellow for warnings/modified, red for errors/deleted,
  cyan for titles/branch names, gray for empty states.
- **Table alignment in log and reflog.** Column-width formatting now separates
  color codes from width calculation, ensuring headers and data columns align
  correctly with wider spacing.
- **Clone writes symlinks correctly.** Clone now creates symlinks via
  `os.Symlink` instead of writing their target paths as regular files.
- **`GetBlobSize` error type unified.** Uses `ErrObjectCorrupted` consistent with
  all other object getters.
- **`PutCommit` existence check.** Added the same early-return from `os.Stat`
  that `PutBlob` and `PutTree` use, preventing unnecessary re-writes.

---

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
