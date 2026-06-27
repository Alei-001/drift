# Color Output System

Drift uses terminal colors to make CLI output more readable. The color system respects user preferences and terminal capabilities.

## Quick Reference

| Color | RGB | Semantic | Examples |
|-------|-----|----------|----------|
| Green | `FgGreen` | Success / confirmation / added | `Saved:`, `A` (status), `Added`, `Restored`, tag names |
| Yellow | `FgYellow` | Warning / modification / caution | `Amended:`, `M` (status), `Skipped`, `Would`, reflog OP column |
| Red | `FgRed` | Error / deletion / aborted | `D` (status), `Aborted`, `Invalid email format` |
| Cyan | `FgCyan` | Headers / branch names / sections | Table headers, `On branch`, `Staged changes:`, `[section]` |
| Gray | `FgHiBlack` | Empty state / no-op / dimmed | `No changes`, untracked files, `Already up to date.` |

## How It Works

Colors are applied by wrapper functions in `internal/cli/color.go`:

```go
func colorGreen(s string) string   // wraps s with green ANSI codes
func colorYellow(s string) string  // wraps s with yellow ANSI codes
func colorRed(s string) string     // wraps s with red ANSI codes
func colorCyan(s string) string    // wraps s with cyan ANSI codes
func colorGray(s string) string    // wraps s with gray (dim) ANSI codes
func colorStatus(s StatusCode) string  // A→green, M→yellow, D→red
```

Each function checks `useColor()` before applying ANSI codes. When color is disabled, the input string is returned unchanged.

## Disabling Colors

Colors are automatically disabled when:

1. `--no-color` flag is set
2. `NO_COLOR` environment variable is set (https://no-color.org/)
3. Stdout is not a terminal (pipe, redirect, or file output)

```bash
# Disable via flag
drift status --no-color

# Disable via environment
NO_COLOR=1 drift status

# Colors auto-disabled when piping
drift status | cat
```

## Porcelain Output

Machine-readable output (`--porcelain` flag) is **never colored**, regardless of terminal settings:

- `drift status --porcelain`
- `drift log --porcelain`
- `drift reflog --porcelain`

This ensures scripts and tools can parse the output without ANSI escape codes.

## Implementation

The color system lives in a single file:

```
internal/cli/color.go   # color functions + useColor() gate
```

All CLI commands call these functions directly when producing output. The `useColor()` gate ensures consistent behavior across all commands — there is no per-command color configuration.

The library `github.com/fatih/color` provides the ANSI code generation. All five color functions use `color.New(...).Sprint(s)` to wrap strings, which only adds ANSI codes when the underlying color output is enabled (matching the `useColor()` logic).

## Adding Colors to New Output

When adding colored output to a new or existing command:

1. Wrap the relevant substring with the appropriate color function
2. For status codes (`A`/`M`/`D`), use `colorStatus()` which maps automatically
3. Never color porcelain output
4. Prefer wrapping individual fragments over entire lines, to keep control characters minimal

```go
// Good: wrap only the fragments that need color
fmt.Printf("Saved: %s (%s)\n", colorGreen(result.ID), result.Message)

// Avoid: wrapping the entire line unnecessarily
fmt.Println(colorGreen(fmt.Sprintf("Saved: %s (%s)", result.ID, result.Message)))
```
