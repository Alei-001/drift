# Drift Architecture Refactoring Plan

## 1. Current Architecture Problems

### 1.1 Root Cause

`internal/repo/Repository` was intended as the single business-logic entry point, but it
only covers a subset of operations: save, switch, restore, branch, name. All read-path
operations — diff, export, history, status — are implemented directly in CLI command
files, bypassing `repo` and talking to `storage.Store` directly.

```
CLI ──直接调用──> storage.Store   (diff.go: 15处, export.go: 全函数, history.go: 8处)
CLI ──部分调用──> repo.Repository  (save, switch, restore 等)
```

### 1.2 Symptom: Four Package-Level Global Variables

```go
// internal/cli/root.go — current
var sharedStore  *storage.Store      // diff.go alone has 15 direct references
var sharedConfig *config.Config
var sharedDir    string
var sharedRepo   *repo.Repository    // partially used, partially bypassed
```

Commands freely choose between `sharedStore.X()`, `sharedRepo.X()`, or `sharedRepo.WT.X()`.

### 1.3 Symptom: Business Logic Scattered in CLI Files

| File | Lines | What's Inside |
|------|-------|---------------|
| `cli/diff.go` | 633 | Myers diff application, blob comparison, file walking, output formatting — all in one file |
| `cli/export.go` | 186 | Directory/ZIP/TAR export with blob writing — all via direct storage calls |
| `cli/history.go` | 317 | Branch listing, commit deduplication, name resolution — all via sharedStore |

### 1.4 Symptom: Dead Code

| File | Line(s) | What |
|------|---------|------|
| `core/progress.go` | 1-22 | `ProgressFunc`, `NullProgress()`, `ConsoleProgress()` — zero callers in entire project |
| `core/diff.go` | 231 | `DiffEditScriptToUnified()` — zero callers |
| `core/diff.go` | 243 | `DiffCountChanges()` — zero callers |

### 1.5 Symptom: Silently Swallowed Errors (Non-Cleanup Paths)

| File | Line | Code | Consequence |
|------|------|------|-------------|
| `cli/status.go` | 18 | `currentBranch, _ := sharedStore.GetRef("HEAD")` | Corrupt ref → silent fallback to "main" |
| `cli/status.go` | 23 | `commit, _ := sharedRepo.CurrentCommit()` | Error swallowed; commit is nil |
| `cli/diff.go` | 354 | `_ = sharedStore.LoadIndex(&idx)` | Index load failure → untracked files not detected |
| `cli/history.go` | 51 | `branch, _ = sharedStore.GetRef("HEAD")` | Corrupt ref → silent fallback to "main" |
| `cli/root.go` | 199 | `line, _ = reader.ReadString('\n')` | Stdin read error → empty name used |
| `cli/rm.go` | 210 | `_ = core.WalkWorkingDirWithIgnore(...)` | Walk error ignored; incomplete cleanup |
| `repo/history.go` | 152 | `_ = r.Store.DeleteRef(change.Ref)` | Undo ref deletion failure → inconsistent state |
| `repo/switch.go` | 226 | `_ = worktree.DeleteWIP(...)` | WIP cleanup failure silently ignored |
| `worktree/worktree.go` | 84 | `_ = os.Chmod(fullPath, perm)` | Permission set failure silently ignored |

---

## 2. Target Architecture

```
cmd/drift/main.go
    │
    │  1. dir = os.Getwd()
    │  2. store = storage.NewStore(dir)
    │  3. cfg = config.LoadConfig(store.DriftDir())
    │  4. app = apppkg.New(store, cfg, dir)
    │  5. rootCmd = cli.BuildRootCmd(app)
    │  6. rootCmd.Execute()
    │
    ▼
┌─────────────────────────────────────────────────────────┐
│ internal/cli/          Presentation Layer                │
│                                                          │
│   Each command file:                                     │
│     - Exports a constructor: NewXxxCmd(app *app.App)     │
│     - Receives *app.App via closure                      │
│     - Responsibilities:                                  │
│       1. Parse CLI flags                                 │
│       2. Validate input                                  │
│       3. Call app method                                 │
│       4. Format output to stdout                         │
│     - Does NOT import storage/, worktree/, config/       │
│     - Only imports: app/ (for types), core/ (for Commit) │
│                                                          │
│   Global state: NONE                                     │
│   No init() side effects besides constructor registration│
└──────────────────────────┬──────────────────────────────┘
                           │ depends on
┌──────────────────────────▼──────────────────────────────┐
│ internal/app/           Application Layer (NEW)          │
│                                                          │
│   App is a CONCRETE struct, not an interface.            │
│   Internally holds: Store, Worktree, Config, Dir         │
│                                                          │
│   Complete method list:                                  │
│                                                          │
│   Lifecycle:                                             │
│     Init() error                                        │
│     IsInitialized() bool                                 │
│     Chdir(dir string) error        — handle --dir flag   │
│                                                          │
│   Staging:                                               │
│     Add(paths []string) (int, error)                     │
│     Unstage(paths []string) (int, error)                 │
│     ClearStaging() error                                │
│                                                          │
│   Commits:                                               │
│     Save(msg string, opts SaveOptions) (*SaveResult)     │
│                                                          │
│   Query:                                                 │
│     History(opts HistoryOptions) ([]*core.Commit, error) │
│     Log(limit int) ([]OperationEntry, error)             │
│     Status() (*core.Status, error)                      │
│     Diff(opts DiffOptions) (*DiffResult, error)          │
│                                                          │
│   Snapshot:                                              │
│     Export(version, output string, fmt ExportFormat)     │
│     Restore(version string) error                       │
│                                                          │
│   Branches:                                              │
│     BranchList() ([]string, error)                       │
│     BranchCreate(name string) error                     │
│     BranchDelete(name string) error                     │
│     BranchRename(old, new string) error                 │
│     CurrentBranch() string                              │
│     Switch(branch string, opts SwitchOpts)(*SwitchResult)│
│                                                          │
│   Tags:                                                 │
│     TagAdd(version, label string) error                 │
│     TagDelete(label string) error                       │
│     TagList() ([]TagEntry, error)                       │
│     TagsByHash() map[string][]string                    │
│                                                          │
│   WIP:                                                   │
│     WIPList(branch string) ([]WIPEntry, error)           │
│     WIPSave(branch string) error                         │
│     WIPRestore(branch string) (int, error)               │
│     WIPDrop(branch string) error                        │
│                                                          │
│   Files:                                                 │
│     Remove(paths []string, opts RemoveOptions) error    │
│     Move(sources []string, dest string, opts MoveOpts)   │
│     Clean(opts CleanOptions) ([]string, error)          │
│                                                          │
│   Undo:                                                  │
│     Undo(count int) (*UndoResult, error)                │
│                                                          │
│   Config:                                                │
│     ConfigGet(scope ConfigScope, key string) (string, er)│
│     ConfigSet(scope ConfigScope, key, val string) error  │
│     ConfigUnset(scope ConfigScope, key string) error    │
│     ConfigList(scope ConfigScope) ([]ConfigEntry, error) │
│                                                          │
│   Sync:                                                  │
│     SyncEnable() error                                  │
│     SyncDisable() error                                 │
│     SyncNow() error                                     │
│     SyncStatus() (*SyncStatus, error)                   │
│     SyncEnabled() bool                                  │
│     SyncRemoteSet(protocol string, opts SyncRemoteOpts) │
│     Clone(remoteName, destDir string) error             │
│                                                          │
│   File split (one file per concern):                     │
│     app.go        — App struct, New(), Author(),          │
│                      ResolveCommit()                       │
│     init.go       — Init, IsInitialized, Chdir            │
│     stage.go      — Add, Unstage, ClearStaging           │
│     commit.go     — Save, computeChangedPaths            │
│     query.go      — History, Status, Log                 │
│     diff.go       — Diff (移植自 cli/diff.go 核心)       │
│     snapshot.go   — Export, Restore                      │
│     switch.go     — Switch, RestoreWIP                   │
│     branch.go     — Branch CRUD                          │
│     tag.go — Tag CRUD, TagsByHash              │
│     wip.go        — WIP list/save/restore/drop            │
│     file.go       — Remove, Move, Clean                  │
│     undo.go       — Undo + OperationEntry types          │
│     config.go     — Config CRUD (local + global)         │
│     sync.go       — Sync operations                     │
└──┬───────┬──────────┬────────────────────────────────────┘
   │       │          │
   ▼       ▼          ▼
core/  storage/  worktree/    ← 基础层，不变
config/  sync/
```

---

## 3. Key Design Decisions

### 3.1 Concrete App Struct, Not Interface

For a ~4000-line project, a single concrete struct is sufficient. Extracting interfaces
before actual mocking needs arise violates YAGNI. The current test strategy (real
filesystem, integration tests) doesn't need interfaces.

If unit-test isolation with mocks becomes necessary later, extract typed service
interfaces (e.g., `StageService`, `DiffService`) from the concrete struct.

### 3.2 Constructor Pattern for Commands

**Before (current):**
```go
var sharedStore *storage.Store
var addCmd = &cobra.Command{
    RunE: func(cmd *cobra.Command, args []string) error {
        sharedStore.LoadIndex(&idx)    // global variable
    },
}
func init() { rootCmd.AddCommand(addCmd) }
```

**After:**
```go
func NewAddCmd(app *apppkg.App) *cobra.Command {
    return &cobra.Command{
        Use: "add <path>...",
        RunE: func(cmd *cobra.Command, args []string) error {
            added, err := app.Add(args)
            if added > 0 { fmt.Printf("Added %d file(s)\n", added) }
            return err
        },
    }
}
```

**Root command assembly:**
```go
func BuildRootCmd(app *apppkg.App) *cobra.Command {
    var (
        globalDir     string
        globalVerbose bool
        globalNoColor bool
    )

    root := &cobra.Command{
        Use: "drift",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Bare "drift": init if needed, else show help.
            if !app.IsInitialized() {
                return app.Init()
            }
            return cmd.Help()
        },
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            if globalDir != "" {
                if err := app.Chdir(globalDir); err != nil { return err }
            }
            // Commands that work without an initialized repository
            switch cmd.Name() {
            case "drift", "init", "clone":
                return nil
            case "config":
                global, _ := cmd.Flags().GetBool("global")
                if global { return nil }
            case "remote":
                if cmd.Parent() != nil && cmd.Parent().Name() == "sync" {
                    return nil  // "drift sync remote"
                }
            }
            if !app.IsInitialized() {
                return fmt.Errorf("not a drift repository (run 'drift init')")
            }
            return nil
        },
    }
    root.PersistentFlags().StringVar(&globalDir, "dir", "", "Repository directory")
    root.PersistentFlags().BoolVar(&globalVerbose, "verbose", false, "Verbose output")
    root.PersistentFlags().BoolVar(&globalNoColor, "no-color", false, "Disable color")

    root.AddCommand(NewInitCmd(app))
    root.AddCommand(NewAddCmd(app))
    // ... all other commands
    return root
}
```

```go
// cmd/drift/main.go
func main() {
    dir, _ := os.Getwd()
    store := storage.NewStore(dir)
    cfg, _ := config.LoadConfig(store.DriftDir())
    app := driftapp.New(store, cfg, dir)

    rootCmd := cli.BuildRootCmd(app)
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

### 3.3 Diff Returns Structured Data, CLI Formats

**Before:** `cli/diff.go` is 633 lines of algorithm + blob I/O + text formatting + file
output all in one file.

**After:**

```go
// internal/app/diff.go
type DiffOptions struct {
    V1, V2 string   // version specifier (empty = working tree)
    Paths  []string // file filter
}
type DiffEntry struct {
    Path     string
    Status   string          // "added", "deleted", "modified"
    IsBinary bool
    OldSize  int64
    NewSize  int64
    Edits    []core.DiffEdit // populated only when showPatch=true
}
type DiffResult struct {
    Entries []DiffEntry
}
func (a *App) Diff(opts DiffOptions) (*DiffResult, error) { ... }
```

```go
// internal/cli/diff.go — reduced to ~60 lines
func NewDiffCmd(app *apppkg.App) *cobra.Command {
    var showPatch bool; var outputFile string; var filePaths []string
    cmd := &cobra.Command{Use: "diff [v1] [v2]", RunE: func(..., args) error {
        v1, v2 := parseVersionArgs(args)
        result, err := app.Diff(apppkg.DiffOptions{V1: v1, V2: v2, Paths: filePaths})
        if err != nil { return err }
        if showPatch { printUnifiedDiff(result) } else { printSummary(result) }
        return nil
    }}
    cmd.Flags().BoolVarP(&showPatch, "patch", "p", false, "...")
    return cmd
}
```

### 3.4 All Config and Sync Operations Go Through App

For consistency, CLI sees only `app`. Config read/write and sync operations delegate
to `config` and `sync` packages internally.

**Config scopes:**
- `LocalScope`: reads/writes `.drift/config.json` via `app.store.DriftDir()`
- `GlobalScope`: reads/writes `~/.drift/global.json` via `sync.LoadGlobalConfig()`

**Global config access without a local repo:**
```go
func (a *App) ConfigGet(scope ConfigScope, key string) (string, error) {
    switch scope {
    case LocalScope:
        if !a.IsInitialized() { return "", fmt.Errorf("not a drift repository") }
        // read from a.config (loaded at construction)
    case GlobalScope:
        gcfg, err := driftsync.LoadGlobalConfig()
        // read from gcfg
    }
}
```

This means `drift config --global user.name` works even without an initialized
repository, because `App` exists (constructed from the current directory) but the
global config path doesn't depend on it.

**Note on `app → sync` dependency**: The global config (`~/.drift/global.json`)
is currently stored and loaded by the `sync` package (`driftsync.LoadGlobalConfig`).
This makes `app` depend on `sync`, which is an inverted layering (business layer
depending on transport layer). This is accepted as-is for this refactoring because
moving global-config storage into `config/` is a separate concern. A future cleanup
could extract `config.LoadGlobal/SaveGlobal` and have both `app` and `sync` depend
on `config/` instead. Tracked as a known debt, not a blocker.

### 3.5 CLI Imports Only app/ and core/

Hard constraint: no CLI file imports `storage`, `worktree`, `config`, or `sync`.

| Allowed import | For |
|---------------|-----|
| `app/` | App type, option structs, result structs |
| `core/` | Commit, DiffEdit, Status (display types) |
| `cobra` | Command framework |
| stdlib | fmt, os, io, etc. |

**Return-type convention**: `core` layer functions may return value types
(e.g. `core.ComputeStatus` returns `core.Status` by value). The `app` layer
uniformly returns **pointers** to these display types (`*core.Status`,
`[]*core.Commit`) so the CLI receives a single, consistent ownership convention
and avoids copying large structs. This is an intentional asymmetry between
layers, not a bug.

### 3.6 Removal of internal/repo/ and .wastebasket/

The entire `internal/repo/` package is moved to `.wastebasket/repo/` — preserved as
coding reference during the transition, deleted after full verification.

### 3.7 Type Migration

| Current Location | Type | New Location |
|-----------------|------|-------------|
| `repo/save.go` | `SaveOptions`, `SaveResult` | `app/commit.go` |
| `repo/switch.go` | `SwitchOptions`, `SwitchResult` | `app/switch.go` |
| `repo/history.go` | `OpType`, `OperationEntry`, `RefChange` | `app/history.go` → renamed to `undo.go` |
| `repo/name.go` | `TagEntry` | `app/tag.go` |
| `repo/repo.go` | `Repository` | Deleted (replaced by `app.App`) |

---

## 4. File-Level Changes

### 4.1 New: internal/app/

```
app.go          App struct, New(), Author(), ResolveCommit()
init.go         Init, IsInitialized, Chdir()
stage.go        Add, Unstage, ClearStaging
commit.go       Save, computeChangedPaths (from repo/save.go)
query.go        History, Status, Log
diff.go         Diff core logic (from cli/diff.go)
snapshot.go     Export, Restore
switch.go       Switch, RestoreWIP (from repo/switch.go)
branch.go       Branch CRUD (from repo/branch.go)
tag.go Tag CRUD, TagsByHash (from repo/name.go)
wip.go          WIP list/save/restore/drop (from repo/wip.go)
file.go         Remove, Move, Clean
undo.go         Undo + OpType/OperationEntry/RefChange (from repo/history.go)
config.go       ConfigGet/Set/Unset/List
sync.go         Sync operations
```

### 4.2 Rewrite: internal/cli/

All command files rewritten from `var xxxCmd = &cobra.Command{...} + init()` to
`func NewXxxCmd(app *app.App) *cobra.Command`.

| File | Target Lines (from) |
|------|-------------------|
| `root.go` | `BuildRootCmd(app)` — ~80 lines (from 191) |
| `add.go` | `NewAddCmd` — ~30 lines (from 50) |
| `save.go` | `NewSaveCmd` — ~40 lines (from 62) |
| `diff.go` | `NewDiffCmd` — ~60 lines (from 633) |
| `export.go` | `NewExportCmd` — ~30 lines (from 186) |
| `history.go` | `NewHistoryCmd` + `NewUndoCmd` — ~80 lines (from 317) |
| `log.go` | `NewLogCmd` — ~30 lines (from 85) |
| `status.go` | `NewStatusCmd` — ~30 lines (from 133) |
| `switch.go` | `NewSwitchCmd` — ~40 lines (from 46) |
| `branch.go` | `NewBranchCmd` — ~40 lines (from 88) |
| `tag.go` | `NewTagCmd` — ~30 lines (from 109) |
| `wip.go` | `NewWIPCmd` — ~40 lines (from 138) |
| `clean.go` | `NewCleanCmd` — ~30 lines (from 65) |
| `rm.go` | `NewRmCmd` — ~50 lines (from 193) |
| `mv.go` | `NewMvCmd` — ~60 lines (from 161) |
| `unstage.go` | `NewUnstageCmd` — ~40 lines (from 106) |
| `clone.go` | `NewCloneCmd` — ~50 lines (from 133) |
| `config.go` | `NewConfigCmd` — ~80 lines (from 309) |
| `sync.go` | `NewSyncCmd` — ~100 lines (from 542) |
| `confirm.go` | unchanged |
| `color.go` | unchanged |

### 4.3 Retire to .wastebasket

| Path | Destination |
|------|------------|
| `internal/repo/` (8 files) | `.wastebasket/repo/` |
| `internal/core/progress.go` | `.wastebasket/core/` |
| `core/diff.go:231-252` (unused functions) | Removed (trivial, no preservation needed) |

### 4.4 Unchanged

| Package | Notes |
|---------|-------|
| `internal/core/` | Data model, hash, codecs, walker, diff algorithm — unchanged structure |
| `internal/storage/` | File system persistence — unchanged |
| `internal/worktree/` | File system operations — unchanged |
| `internal/config/` | JSON config read/write — unchanged |
| `internal/sync/` | Transport implementations — logic unchanged |
| `cmd/drift/main.go` | Rewritten to use BuildRootCmd |

---

## 5. Test Migration

### Before (current)
```go
func TestAdd(t *testing.T) {
    h := NewTestHelper(t)
    defer h.Cleanup()
    h.SetupSharedState()     // sets 4 package-level globals
    defer resetAllFlags()     // resets ~30 flag variables
    out, _ := h.RunAdd("file.txt")
    h.AssertContains(out, "Added")
}
```

### After
```go
func TestAdd(t *testing.T) {
    dir := t.TempDir()
    store := storage.NewStore(dir)
    store.Init()
    cfg, _ := config.LoadConfig(store.DriftDir())
    app := apppkg.New(store, cfg, dir)

    cmd := cli.NewAddCmd(app)
    cmd.SetArgs([]string{"file.txt"})
    output := captureOutput(func() { cmd.Execute() })
    assertContains(t, output, "Added")
}
```

No `SetupSharedState`, no `resetAllFlags`, no test helper globals.

---

## 6. Execution Plan

| Phase | Content | Validation |
|-------|---------|------------|
| **P1** | Create `internal/app/app.go` — App struct, New(), all method signatures | `go build ./internal/app/...` |
| **P2** | Implement app methods: port from repo/ + extract from cli/ | `go build ./...` |
| **P3** | Create `cli.BuildRootCmd(app)` + `Chdir` for --dir flag | `go build ./...` |
| **P4** | Rewrite cli commands one-by-one (constructor pattern) | After each: `go test ./...` |
| **P5** | Move old code to `.wastebasket/`, rename `*_new.go` → `*.go`, remove dead functions | `go build ./...` |
| **P6** | Migrate tests: replace TestHelper with direct App creation | `go test ./...` |
| **P7** | Clean Chinese text in Go comments/help strings | `go build ./...` |

Each phase is a separate commit. No phase breaks existing functionality (P1-P3 are
pure additions).

---

## 7. Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Large diff hard to review | Phased commits: P1-P2 are NEW files, no existing code modified |
| Test breakage | Rewrite one CLI file → run tests → commit. Fail fast, revert single file |
| Method signature mismatch | All signatures defined in `app/app.go` first, implemented second |
| Unused imports after refactor | `goimports -w` after each CLI file rewrite |
| Pre-init command behavior change | `PersistentPreRunE` explicitly skips `IsInitialized` check for bare `drift`, `init`, `clone`, `config --global`, and `sync remote` |
| `--dir` flag chicken-and-egg | App is created in `main.go` with cwd; `--dir` in `PersistentPreRunE` calls `app.Chdir()` to re-initialize store/config at the new directory |

---

## 8. Benefits

1. **Zero global state in CLI** — each command only has `app` in closure
2. **Clear boundaries** — CLI = parse + format, App = all business logic
3. **diff/export/history usable without cobra** — e.g. from a GUI or script
4. **Backend swappable** — replace App implementation (in-memory test, SQLite)
5. **Test isolation** — each test creates its own App, no shared state to reset
6. **Natural error handling** — App methods return errors; no silent swallowing
