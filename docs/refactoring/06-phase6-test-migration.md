# Phase 6: Test Migration

## Goal

Rewrite all test files to use the new constructor-based command registration and
direct App creation, replacing `TestHelper.SetupSharedState()` + `resetAllFlags()`.

## Verification

```bash
go test ./...              # all tests pass
```

## Tasks

### 6.1 — Create new TestHelper

**File**: `internal/cli/cli_test.go`

Replace current `TestHelper` struct and methods:

```go
type TestHelper struct {
    Dir string
    T   *testing.T
    App *apppkg.App
}

func NewTestHelper(t *testing.T) *TestHelper {
    t.Helper()
    dir := t.TempDir()
    store := storage.NewStore(dir)
    if err := store.Init(); err != nil {
        t.Fatal(err)
    }
    cfg, _ := config.LoadConfig(store.DriftDir())
    app := apppkg.New(store, cfg, dir)
    return &TestHelper{Dir: dir, T: t, App: app}
}

// No Cleanup() needed — t.TempDir() auto-cleans
// No SetupSharedState() needed
// No resetAllFlags() needed
```

### 6.2 — Rewrite TestHelper command runners

Each `RunXxx()` method creates a command via `NewXxxCmd(app)`, sets args, captures output:

```go
func (h *TestHelper) RunAdd(args ...string) (string, error) {
    cmd := NewAddCmd(h.App)
    cmd.SetArgs(args)
    return CaptureOutput(func() error {
        return cmd.Execute()
    })
}

func (h *TestHelper) RunSave(args ...string) (string, error) {
    cmd := NewSaveCmd(h.App)
    cmd.SetArgs(args)
    return CaptureOutput(func() error {
        return cmd.Execute()
    })
}
// ... all RunXxx methods
```

### 6.3 — Update CaptureOutput

Keep existing `CaptureOutput` helper. It redirects `os.Stdout` — acceptable for tests.

### 6.4 — Migrate test files

For each test file, replace old patterns:

**Before:**
```go
func TestFoo(t *testing.T) {
    h := NewTestHelper(t)
    defer h.Cleanup()
    h.SetupSharedState()
    defer resetAllFlags()
    out, _ := h.RunAdd("file.txt")
}
```

**After:**
```go
func TestFoo(t *testing.T) {
    h := NewTestHelper(t)
    out, _ := h.RunAdd("file.txt")
}
```

Files to migrate:

| File | Commands tested | Notes |
|------|----------------|-------|
| `cli_test.go` | TestHelper itself | Rewrite TestHelper + helper methods |
| `workflow_test.go` | add, save, history, export, switch, restore | Replace `RunLogAll` → `RunHistoryAll` |
| `staging_test.go` | add, unstage | |
| `log_test.go` | log, undo | |
| `diff_test.go` | diff | Replace version IDs (v1 → hash-based ID) |
| `export_restore_test.go` | export, restore | |
| `error_edge_test.go` | various error paths | |
| `config_ignore_test.go` | config, .driftignore | |
| `rm_mv_test.go` | rm, mv | |
| `name_test.go` | name | |
| `version_test.go` | version resolution | Replace v1/v2 references |
| `history_test.go` | history | |
| `wip_test.go` | wip | |
| `clean_test.go` | clean | |

### 6.5 — Delete resetAllFlags()

Remove `resetAllFlags()` function from `cli_test.go`. No longer needed — each test creates fresh commands.

### 6.6 — Handle version ID changes

Some tests reference old sequential version IDs (`v1`, `v2`). The new system uses abbreviated hash IDs (8 hex chars). Tests that use `ExtractSaveID()` or similar helpers from TestHelper should be updated.

Ensure TestHelper has:
- `ExtractSaveID(output string) string` — parses commit ID from save output
- `AssertContains(t, output, substr)`
- `AssertNoError(t, err)`

### 6.7 — Full test pass

```bash
go test ./... -count=1
```

Fix any failures. Expected failure sources:
- Hardcoded `v1`/`v2` references → replace with `h.ExtractSaveID(output)`
- `RunLogAll()` → rename to `RunHistoryAll()`
- Missing test helper methods → add as needed

## Deliverables

- All test files updated to use `NewTestHelper(t)` + constructor pattern
- No `SetupSharedState()`, no `resetAllFlags()`, no global state in tests
- `go test ./...` all green
- Tests are parallel-safe (no shared package-level state)
