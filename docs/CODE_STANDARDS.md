# Drift code standards

This document defines the code conventions for the **drift** project. It is the reference for both human contributors and AI-assisted edits — any change that violates these conventions is a defect regardless of functional correctness.

---

## 1. Naming

### 1.1 Acronyms

All uppercase for initialisms in identifiers — `ID`, `URL`, `HTTP`, `FS`, `MIME`:

```
✅ configID, fsPath, HTTPClient, MIMEType
❌ configId, fsUrl, HttpClient, MimeType
```

### 1.2 Receivers

One or two characters reflecting the type name:

```
✅ func (fs *FSStorage) ...
✅ func (m *MemoryStorage) ...
✅ func (s *Snapshot) ...
❌ avoid descriptive names like (store *FSStorage)
```

The project uses `fs` for `FSStorage` and `ms` for `MemoryStorage`. Either is acceptable, but **pick one per file and stay consistent**.

### 1.3 Interface naming

Single-method interfaces end in `-er`: `Chunker`, `Storer`, `Detector`, `Differ`.

Multi-method composite interfaces (like `Storer`) use `-er` suffix. No hard rule on naming — just be consistent with what already exists.

### 1.4 Exported vs unexported

Everything exported must have a doc comment starting with the name:

```go
// Hash is a BLAKE3 hash (32 bytes).
type Hash [32]byte
```

---

## 2. Error handling

### 2.1 Sentinel errors

Define sentinel errors in the package that owns the concept:

| Package | Sentinel errors |
|---------|----------------|
| `internal/storage/` | `ErrNotFound`, `ErrAlreadyExists`, `ErrInvalidRef`, `ErrCorrupted`, `ErrUnsupported` |
| `internal/porcelain/` | `ErrNothingToSave`, `ErrBranchNotFound`, `ErrBranchAlreadyExists`, `ErrSnapshotNotFound`, `ErrTagAlreadyExists`, `ErrLocked`, `ErrCannotDeleteCurrentBranch`, `ErrCannotDeleteMain`, `ErrCannotRenameMain` |

Message format: either prefixed (`drift: not found`, storage/) or plain (`nothing to save`, porcelain/). Be consistent within a package.

```go
// internal/storage/errors.go
var ErrNotFound = errors.New("drift: not found")
```

### 2.2 Wrapping

Always use `fmt.Errorf("...: %w", err)` when adding context. Never use `%v` for errors — it loses the chain.

```
✅ return fmt.Errorf("open storage: %w", err)
❌ return fmt.Errorf("open storage: %v", err)
❌ return err  // only when the caller context adds nothing
```

### 2.3 Classifying errors

Use `errors.Is()` and `errors.As()`, never string matching:

```
✅ if errors.Is(err, storage.ErrNotFound) { ... }
❌ if strings.Contains(err.Error(), "not found") { ... }
```

Production code must classify errors with `errors.Is` / `errors.As`. Test code is exempt: tests may use `strings.Contains(err.Error(), ...)` to assert user-facing error messages.

### 2.4 Silent error discarding

Only discard errors when there is a documented reason why the operation is best-effort: the `_ =` must appear on its own line with a comment:

```go
// Best-effort: dir may not support sync (Windows).
_ = d.Sync()
```

---

## 3. Defensive programming

### 3.1 Nil checks on interface returns

Any function that returns an `interface{...}` and documents that nil is possible must have its return value checked before calling methods:

```go
engine := filetype.DetectEngine(path, header)
if engine == nil {
    engine = &binary.BinaryEngine{} // safe fallback
}
// now safe to call engine.ChunkerFor(...)
```

### 3.2 Type assertions

Always use the comma-ok pattern:

```
✅ if fsStore, ok := store.(*filesystem.FSStorage); ok { ... }
❌ fsStore := store.(*filesystem.FSStorage)  // panics on mismatch
```

### 3.3 Resource cleanup

Use `defer` immediately after resource acquisition:

```
✅ f, err := os.Open(path)
   if err != nil { ... }
   defer f.Close()
```

---

## 4. Magic numbers

### 4.1 Named constants required

All non-trivial literals must be named constants. Existing examples:

- `core.HeaderPeekSize = 512` (header peek buffer size)
- `storage.MaxSymRefDepth = 8` (maximum symbolic reference chain depth)
- `storage.MaxChunkMinSize` / `MaxChunkAvgSize` / `MaxChunkMaxSize` (chunk size bounds)

---

## 5. Tests

### 5.1 Seam

All tests verify behavior through the public interface. No test accesses unexported fields via `reflect` or `unsafe`.

```
✅ Test that a FixedChunker produces chunks ≤ chunkSize
❌ Test that chunker.chunkSize == 4096 (reflect on private field)
```

### 5.2 Assertions

Use the standard library `testing` package only. No testify, gomega, or other frameworks.

Value assertions compare against **independent, known-good literals** — not recomputed values:

```
✅ func TestSizeFormat(t *testing.T) {
       if got := formatSize(2048); got != "2.0 KB" { ... }
   }
❌ func TestSizeFormat(t *testing.T) {
       expected := fmt.Sprintf("%.1f KB", float64(2048)/1024)
       if got := formatSize(2048); got != expected { ... }
   }
```

### 5.3 Test naming

`TestFunctionName_Scenario`:

```
TestCreateBranch_FromHead
TestCreateBranch_AlreadyExists
TestCreateBranch_InvalidName
```

### 5.4 Test backend

Prefer `internal/storage/backends/memory.MemoryStorage` over temp directories for porcelain tests.

---

## 6. Code organization

### 6.1 File size

Aim for ≤ 300 lines per file. Split by concern when exceeding this.

### 6.2 Package layout

```
cmd/                  — CLI commands and display formatting (NO business logic)
  drift/              — main binary entry point
internal/             — business implementation (not importable externally)
  porcelain/          — business logic (snapshot, branch, restore, lock, watch, GC)
  core/               — domain types, interfaces, protobuf codec
  storage/            — Storer interface + sentinel errors + shared clone helpers
    backends/         — storage implementations (interface/impl physically separated)
      filesystem/     — on-disk implementation (.drift/)
      memory/         — in-memory implementation (for tests)
    refname/          — reference name validation
    stream/           — chunk streaming helpers
  chunker/            — content-defined chunking strategies
  filetype/           — engine interface, registry
    text/             — text engine (detection, diff, preview)
    image/            — image engine
    video/            — video engine
    binary/           — binary fallback engine
  util/               — generic utilities
    cache/            — LRU cache wrapper
    fsutil/           — filesystem helpers (atomic writes, walk)
    glob/             — glob pattern matching
    pathutil/         — cross-platform path normalization
    format/           — size/dimension formatting
```

The `internal/` boundary enforces the layer order: external projects cannot
import any business package, so the only public surface is the CLI.

### 6.3 De-duplication rule

Any function or constant that appears identically in two files must be extracted to the nearest shared ancestor package.

---

## 7. Documentation

### 7.1 Exported types

Every exported type (struct, interface, type alias) must have a doc comment:

```go
// ChunkFlag marks whether a chunk is compressed.
type ChunkFlag uint8

// Chunk is a content-addressed chunk of file data.
type Chunk struct {
    Hash  Hash      // BLAKE3 content hash
    Size  uint32    // uncompressed size in bytes
    Data  []byte    // raw bytes (may be compressed)
    Flags ChunkFlag // compression and encoding flags
}
```

### 7.2 Commands

All cobra commands must define `Use`, `Short`, and `Long` fields.

---

## 8. Security

### 8.1 Path validation

All user-provided paths entering the system must pass through `internal/util/pathutil.RelToWorkDir` before any filesystem operation.

### 8.2 Tag and branch names

All reference names must be validated via `internal/storage/refname.Validate()` before storage:

```go
if err := refname.Validate("tags/" + name); err != nil {
    return fmt.Errorf("invalid tag name: %w", err)
}
```

### 8.3 External process execution

Any `exec.Command` must use only program-generated or hardcoded arguments. User input must never appear directly in `exec.Command` args.

### 8.4 File extension allowlist for preview

The `safePreviewExts` map in `cmd/show.go` is the single source of truth for which file types can be handed to the system viewer.

---

## 9. Import style

Standard library first, then third-party, then project-internal. Blank line between groups.

---

## 10. Context cancellation

All loops that span potentially large data structures must periodically check `ctx.Err()`:

```go
for _, item := range items {
    if err := ctx.Err(); err != nil {
        return err
    }
    // process item
}
```

---

## Changelog

| Date | Change |
|------|--------|
| 2026-07-02 | Initial version — codifies review findings and existing conventions |
