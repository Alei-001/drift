# Phase 4: Command Rewrite

## Goal

For each CLI command, create a `*_new.go` file with a `NewXxxCmd(app)` constructor.
Register each new command in `BuildRootCmd`. Old files remain untouched for test compatibility.

Coexistence approach:
- `add.go` (old) + `add_new.go` (new) — both compile, old init() registers on stale rootCmd
- `BuildRootCmd` registers `NewAddCmd(app)` on the new rootCmd
- `go build && go test` pass at every step
- Old files will be moved to `.wastebasket/cli/` in P5 (not deleted)

**Important constraints during P4:**

1. **Keep `resetAllFlags()` and all global flag vars intact.** Old tests still
   reference them and old `init()` functions still register flags on the stale
   `rootCmd`. These are only removed in P6. Do NOT delete them early.

2. **Avoid private-helper name collisions.** Each `*_new.go` file lives in the
   same `cli` package as the old `.go` file. If both define a top-level helper
   with the same name (e.g. `func printSummary(...)`), compilation fails. When
   porting a helper, either:
   - reuse the old one (don't re-declare it in `*_new.go`), or
   - rename the new version (e.g. `printDiffSummary`), or
   - make it a method on a local formatter struct inside `NewXxxCmd`.
   Run `go build ./...` after EACH `*_new.go` is added to catch this early.

## Verification (per command)

```bash
go build ./... && go test ./...
# Then manual smoke test:
go run ./cmd/drift/ <command> --help
```

## Tasks

### 4.1 — Init

**Create**: `internal/cli/init_new.go`

```go
func NewInitCmd(app *apppkg.App) *cobra.Command {
    return &cobra.Command{
        Use:   "init",
        Short: "Initialize a new drift repository",
        RunE: func(cmd *cobra.Command, args []string) error {
            return app.Init()
        },
    }
}
```

Register in `root_new.go`: `root.AddCommand(NewInitCmd(application))`

### 4.2 — Add

**Create**: `internal/cli/add_new.go`

- Receive raw args from command line
- Call `app.Add(args)` — app handles glob expansion internally; returns (added int, error)
- Print "Added N file(s)" on success

### 4.3 — Unstage

**Create**: `internal/cli/unstage_new.go`

- No args → `app.ClearStaging()` (with confirmation)
- With args → `app.Unstage(args)` — app handles path expansion internally

### 4.4 — Save

**Create**: `internal/cli/save_new.go`

- Parse `-m/--message`, `--amend`, `--all`, `--name` flags
- Call `app.Save(message, apppkg.SaveOptions{...})`
- Print save result with version ID and staged paths

### 4.5 — History

**Create**: `internal/cli/history_new.go`

- Parse `--all`, `--oneline`, `-n`, `--porcelain` flags
- Call `app.History(apppkg.HistoryOptions{...})` — returns `[]*core.Commit`
- Format output in CLI (table/oneline/porcelain)
- Name alias resolution via `app.NamesByHash()`

### 4.5b — Undo

**Create**: in `internal/cli/history_new.go` (same file as History)

- Parse `-n/--number` flag (number of operations to undo, default 1)
- Call `app.Undo(n)` — returns `*app.UndoResult`
- Print undone operation description and ref count

Both `NewHistoryCmd` and `NewUndoCmd` exported from `history_new.go`.
Register both in `BuildRootCmd`: `root.AddCommand(NewHistoryCmd(app))`, `root.AddCommand(NewUndoCmd(app))`.

### 4.6 — Log

**Create**: `internal/cli/log_new.go`

- Parse `-n`, `--porcelain` flags
- Call `app.Log(limit)` — returns `[]app.OperationEntry`
- Print formatted list

### 4.7 — Status

**Create**: `internal/cli/status_new.go`

- Call `app.Status()` — returns `*core.Status`
- Print formatted status table
- **Must NOT call `core.ComputeStatus` directly** — that reaches into
  `storage.Store` and `worktree`, violating the "CLI imports only app/ + core/"
  rule. `ComputeStatus` is now invoked inside `app.Status()` (see app/query.go).
- The CLI's only job is formatting the `*core.Status` struct returned by App.

### 4.8 — Diff

**Create**: `internal/cli/diff_new.go`

The biggest rewrite. Currently 633 lines → target ~60 lines.

- Parse version args, `-p/--patch`, `-o/--output`, `-f/--file` flags
- Call `app.Diff(apppkg.DiffOptions{V1, V2, Paths})` — returns `*app.DiffResult`
- If `-p`: print unified diff (using `core.DiffEdit` for line-level)
- If no `-p`: print summary table with +/- stats
- If `-o`: write to file instead of stdout
- Binary file detection in CLI (small helper)

Old `cli/diff.go` remains untouched (provides `var diffCmd` for test compilation).
The core diff logic was extracted to `app/diff.go` in P2.8.

### 4.9 — Export

**Create**: `internal/cli/export_new.go`

- Parse version, `-o/--output`, `-f/--format` flags
- Call `app.Export(version, output, apppkg.ExportFormat(format))`
- No blob I/O in CLI

### 4.10 — Restore

**Create**: `internal/cli/restore_new.go`

- Parse version from args
- Call `app.Restore(version)`
- Print restored message

### 4.11 — Switch

**Create**: `internal/cli/switch_new.go`

- Parse branch name, `--force`, `-c/--create` flags
- Call `app.Switch(branch, apppkg.SwitchOptions{...})` — returns `*app.SwitchResult`
- Print created/WIP-restored messages

### 4.12 — Branch

**Create**: `internal/cli/branch_new.go`

- Parse name, `-d/--delete`, `-m/--move` flags
- Delegate to `app.BranchCreate/Delete/Rename/List`
- Print output

### 4.13 — Name

**Create**: `internal/cli/name_new.go`

- Parse label, version, `-d/--delete`, `--list` flags
- Delegate to `app.NameAdd/Delete/List`
- Print output

### 4.14 — Rm

**Create**: `internal/cli/rm_new.go`

- Parse raw args, `--cached`, `--recursive`, `-f/--force`
- Delegate to `app.Remove(args, apppkg.RemoveOptions{...})` — app handles path/glob expansion internally
- Tracked-paths resolution done inside app.Remove()

### 4.15 — Mv

**Create**: `internal/cli/mv_new.go`

- Parse sources + dest, `-f/--force`
- Delegate to `app.Move(sources, dest, apppkg.MoveOptions{...})`
- Tracked-paths resolution done inside app.Move()

### 4.16 — WIP

**Create**: `internal/cli/wip_new.go`

Subcommands as a `wipCmd` with child commands:

- `wip list [branch]` → `app.WIPList(branch)` — list saved WIP entries
- `wip save [branch]` → `app.WIPSave(branch)` — save current staging as WIP
- `wip restore [branch]` → `app.WIPRestore(branch)` — restore WIP to staging
- `wip drop [branch]` → `app.WIPDrop(branch)` — delete saved WIP

Each subcommand delegates to its corresponding `app.WIP*()` method.

### 4.17 — Clean

**Create**: `internal/cli/clean_new.go`

- Parse `-n/--dry-run`, `-d/--dirs`, `-f/--force`
- Delegate to `app.Clean(apppkg.CleanOptions{...})`
- Print removed file list

### 4.18 — Clone

**Create**: `internal/cli/clone_new.go`

- Parse project name, optional destination
- Delegate to `app.Clone(remoteName, destDir)`
- Print next-steps message

### 4.19 — Config

**Create**: `internal/cli/config_new.go`

- Parse `--global`, `--list`, `--unset`, key-value pairs
- Delegate to `app.ConfigGet/Set/Unset/List`
- Determine scope: `apppkg.LocalScope` or `apppkg.GlobalScope`

### 4.20 — Sync

**Create**: `internal/cli/sync_new.go`

- Subcommands: remote, enable, disable, now, status
- Each delegates to `app.Sync*()` method
- `sync remote` → `app.SyncRemoteSet()`
- `sync now` → `app.SyncNow()`

### 4.21 — Mark old files for retirement

Old files remain untouched in P4 (no modification). They coexist with `*_new.go` for compilation compatibility.

Create a `.wastebasket/` directory at repo root:

```bash
mkdir -p .wastebasket/cli
```

Add `.wastebasket/` to `.gitignore` (already tracked files will be excluded from now on).

## Deliverables

- `mkdir -p .wastebasket/cli` — ready to receive old files in P5
- 20 `*_new.go` files with 21 constructor functions (history_new.go exports 2: NewHistoryCmd + NewUndoCmd)
- Old `.go` files untouched (still provide `var xxxCmd` for test compilation)
- `root_new.go` with all 21 `root.AddCommand(NewXxxCmd(app))` registrations
- `go build ./... && go test ./...` pass
