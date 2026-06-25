# Phase 7: Polish

## Goal

Clean up remaining issues: Chinese text in Go files, go vet warnings, final validation.

## Verification

```bash
go vet ./...
go test ./...
go build -o dist/drift ./cmd/drift/
```

## Tasks

### 7.1 — Remove Chinese text from non-test .go files

After P5, the old command files live in `.wastebasket/cli/` (gitignored, not
shipped). The active `internal/cli/*.go` files are the rewritten `*_new.go`
(now renamed to `*.go`). So this step is a **fresh scan** of the current source
tree, not a fixed line-number checklist.

**Step 1 — Scan all active non-test .go files:**

```bash
rg '[\x{4e00}-\x{9fff}]' internal/ cmd/ --glob '!*_test.go' --glob '!.wastebasket/**'
```

Any match must be replaced with an English equivalent. Common locations to
check (historically contained Chinese, may or may not still apply after rewrite):

| Area | Historical content | English replacement |
|------|--------------------|---------------------|
| `internal/sync/webdav.go` | `// 坑云 WebDAV` comment | `// Nutstore WebDAV` |
| `internal/cli/diff.go` help text | `章节/第一章.txt` example paths | `chapter/chapter1.txt` |
| `internal/cli/sync.go` help text | `坑云` in help | `Nutstore` |
| `internal/cli/name.go` help/comments | `v1 客户终稿` | `v1 final-draft` |

If the rewrite in P4 already used English in these spots, no change is needed —
the scan will come back empty and that's the desired outcome.

**Step 2 — Verify scan returns empty:**

```bash
rg '[\x{4e00}-\x{9fff}]' internal/ cmd/ --glob '!*_test.go' --glob '!.wastebasket/**'
# Must return empty
```

**Files to skip** (test data, not user-facing):

| File | Reason |
|------|--------|
| `internal/cli/*_test.go` | Test content data |
| `.wastebasket/**` | Archived old code, not shipped |

### 7.2 — Run go vet

```bash
go vet ./...
```

Fix all warnings. Common vet issues to expect:
- Unreachable code after return statements
- Unkeyed composite literals
- Missing error format verbs
- Unused variables introduced during rewrite

### 7.3 — Run full test suite

```bash
go test ./... -count=1 -race
```

- `-race` flag: verify no data races (new architecture should be race-free since tests don't share state)
- `-count=1`: disable test caching

### 7.3a — Output snapshot regression check

Unit tests assert substrings, which can miss subtle output drift (column widths,
wording, ordering). This step compares the rewritten binary's command output
against a snapshot recorded from the pre-refactor binary.

**Pre-condition**: A "golden" snapshot must be captured **before** P5 retires
the old code. Ideally record it at the start of P4 (or now, if not yet done),
while the old `internal/cli/` still compiles and runs:

```bash
# 1. Build the PRE-refactor binary from the current (old) codebase
go build -o dist/drift-old ./cmd/drift/

# 2. Run a fixed scenario script and save outputs
#    Scenario: init -> add -> save -> history -> status -> diff
./dist/drift-old init
echo "hello" > a.txt
./dist/drift-old add a.txt
./dist/drift-old save -m "first"
./dist/drift-old history --porcelain   >  snapshot_history.txt
./dist/drift-old status --porcelain     >  snapshot_status.txt
./dist/drift-old diff                   >  snapshot_diff.txt
./dist/drift-old log --porcelain        >  snapshot_log.txt
# Store these snapshots in docs/refactoring/snapshots/ (commit them)
```

**In P7**, after building the new binary, replay the same scenario and diff:

```bash
go build -o dist/drift ./cmd/drift/
# Re-run the identical scenario in a fresh temp dir, then:
diff snapshot_history.txt new_history.txt   # expect: empty
diff snapshot_status.txt   new_status.txt   # expect: empty
diff snapshot_diff.txt     new_diff.txt     # expect: empty
diff snapshot_log.txt      new_log.txt       # expect: empty
```

**Tolerated differences** (document and accept, do not treat as regressions):
- Help text (`--help`) wording changes are OK — the rewrite intentionally
  simplifies help strings.
- Version ID format: if hash-based IDs replaced sequential `v1/v2`, commit
  identifiers in `history`/`log` output will differ in content but must match
  in **format** (same column structure, same number of fields).
- Error message wording may improve (C3 conventions); only flag regressions
  where an error that used to be reported is now silent, or vice versa.

**Untolerated differences** (must investigate):
- `status --porcelain` gaining/losing a row for the same working-tree state
- `diff` showing a different set of changed files for the same tree pair
- `history` listing a different commit count or order for the same branch
- Any command that previously exited 0 now exits non-zero (or vice versa)

If snapshots were not captured before P5, skip this step but record the gap in
`AGENTS.md` as a known verification limitation.

### 7.4 — Build final binary

```bash
go build -o dist/drift ./cmd/drift/
```

Smoke test:
```bash
./dist/drift --help
./dist/drift init
./dist/drift add --help
./dist/drift save --help
./dist/drift history --help
./dist/drift status --help
./dist/drift diff --help
# Spot-check: init + add + save + history + status in a temp dir
```

### 7.5 — Update AGENTS.md

If `AGENTS.md` references old architecture (e.g., `internal/repo/`, `sharedStore`), update:

- Replace package table entry for `internal/repo` with `internal/app`
- Update "Global mutable state" warning
- Update testing section

### 7.6 — Final sanity check

```bash
# No remaining references to deleted packages (exclude docs, markdown, wastebasket)
rg 'internal/repo' --glob '!docs/**' --glob '!*.md' --glob '!.wastebasket/**'
# Should return empty

# No storage imports in active cli/ (exclude test files)
rg '"github.com/drift/drift/internal/storage"' internal/cli/ --glob '!*_test.go'
# Should return empty

# No worktree imports in active cli/
rg '"github.com/drift/drift/internal/worktree"' internal/cli/ --glob '!*_test.go'
# Should return empty

# No config imports in active cli/
rg '"github.com/drift/drift/internal/config"' internal/cli/ --glob '!*_test.go'
# Should return empty
```

## Deliverables

- All .go files (non-test) free of Chinese text
- `go vet ./...` zero warnings
- `go test ./... -race` all green
- `dist/drift` binary functional
- `AGENTS.md` updated to reflect new architecture
