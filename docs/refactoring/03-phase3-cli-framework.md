# Phase 3: CLI Framework

## Goal

Create the new command registration framework (`BuildRootCmd`), wire it to `main.go`.
Old code remains intact and functional during transition.

## Verification

```bash
go build ./...
go run ./cmd/drift/ --help     # shows new command tree
go run ./cmd/drift/ init       # creates .drift/ in current dir
```

## Tasks

### 3.1 — Create BuildRootCmd

**File**: `internal/cli/root_new.go`

```go
package cli

import (
    "fmt"
    "github.com/drift/drift/internal/app"
    "github.com/spf13/cobra"
)

func BuildRootCmd(application *app.App) *cobra.Command {
    var (
        globalDir     string
        globalVerbose bool
        globalNoColor bool
    )

    root := &cobra.Command{
        Use:   "drift",
        Short: "A creative version control system",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Bare "drift" with no subcommand: initialize if needed, else show help.
            if !application.IsInitialized() {
                return application.Init()
            }
            return cmd.Help()
        },
        PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
            // Handle --dir flag
            if globalDir != "" {
                if err := application.Chdir(globalDir); err != nil {
                    return err
                }
            }

            // Commands that work without an initialized repository
            switch cmd.Name() {
            case "drift", "init", "clone":
                // "drift" = root command (bare, no subcommand — runs init)
                // "init"  = drift init subcommand
                // "clone" = drift clone subcommand
                return nil
            case "config":
                global, _ := cmd.Flags().GetBool("global")
                if global {
                    return nil
                }
            case "remote":
                // "drift sync remote" — parent is "sync", no repo needed
                if cmd.Parent() != nil && cmd.Parent().Name() == "sync" {
                    return nil
                }
            }

            if !application.IsInitialized() {
                return fmt.Errorf("not a drift repository (run 'drift init')")
            }
            return nil
        },
    }

    // Persistent flags (same as current)
    root.PersistentFlags().StringVar(&globalDir, "dir", "", "Repository directory")
    root.PersistentFlags().BoolVar(&globalVerbose, "verbose", false, "Verbose output")
    root.PersistentFlags().BoolVar(&globalNoColor, "no-color", false, "Disable color output")

    // Register commands (stubs in P3, live in P4)
    // root.AddCommand(NewInitCmd(application))
    // root.AddCommand(NewAddCmd(application))
    // ... register as created in P4

    return root
}
```

**Maintaining the `PersistentPreRunE` allowlist**: The `switch cmd.Name()` block
hardcodes the commands that may run without an initialized repo (`drift`, `init`,
`clone`, `config --global`, `sync remote`). **When adding a new command that
also works pre-init, you must add a case here** — otherwise users get a spurious
"not a drift repository" error.

A more extensible future design would mark commands via
`cmd.Annotations["requires-init"] = "false"` and check that annotation here
instead of a name switch. That is left as a post-refactor improvement; for now,
treat this switch as a maintenance touchpoint and update it whenever a new
pre-init command is added. Add a comment in the switch:

```go
// MAINTENANCE: when adding a new command that works without an initialized
// repository, add its case here. See docs/refactoring/03-phase3-cli-framework.md.
```

### 3.2 — Update main.go

**Step 1**: Archive old main.go for reference:

```bash
mkdir -p .wastebasket/cmd
cp cmd/drift/main.go .wastebasket/cmd/main.go
```

**File**: `cmd/drift/main.go`

```go
package main

import (
    "os"

    "github.com/drift/drift/internal/app"
    "github.com/drift/drift/internal/cli"
    "github.com/drift/drift/internal/config"
    "github.com/drift/drift/internal/storage"
)

func main() {
    dir, _ := os.Getwd()
    store := storage.NewStore(dir)
    cfg, _ := config.LoadConfig(store.DriftDir())

    application := app.New(store, cfg, dir)
    rootCmd := cli.BuildRootCmd(application)

    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

`--dir` is handled entirely by cobra's `PersistentPreRunE` (calls `app.Chdir()`).
No manual arg parsing needed — simpler and avoids duplicate flag handling.

**Note**: The `App.Author()` method (implemented in P2) handles the fallback chain:
project-level `config.User` → global `~/.drift/global.json`. main.go does not need
to pre-load the global config.

### 3.3 — Verify framework

```bash
go build ./...
./dist/drift --help          # should show skeleton command tree
./dist/drift init            # should create .drift/
```

At this point, commands in `BuildRootCmd` may have no implementations registered (stubs). The old package-level `rootCmd` still exists (old `init()` functions still register old commands on it), but it is **no longer invoked** — `main.go` now calls `BuildRootCmd(app).Execute()`, so only the new root command tree is live.

**Critical**: Before P3, no `BuildRootCmd` usage; after P3, `main.go` uses `BuildRootCmd`. But `BuildRootCmd` has no subcommands registered yet (P4 adds them). Result: `drift --help` shows an empty command tree; bare `drift` runs `app.Init()` via the root `RunE`. The old commands are effectively dormant during P3 — they still compile (so `go test` passes) but are unreachable from the binary. This is expected and is exactly why P4 must register every command before P5 retires the old files.

## Deliverables

- `internal/cli/root_new.go` — `BuildRootCmd(app)` with flag handling and init check
- `cmd/drift/main.go` — rewired to use `BuildRootCmd`
- Old `internal/cli/root.go` unchanged (provides backward compat for old commands)
