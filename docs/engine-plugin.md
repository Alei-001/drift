# File Type Engine Plugin Guide

drift uses a pluggable engine system to handle different file types (text,
image, video, binary). Each engine autonomously controls chunking strategy,
diff behavior, preview generation, and metadata — porcelain never switches
on engine names.

## Engine Interface

Every engine implements the `Engine` interface
([`internal/filetype/engine.go`](../internal/filetype/engine.go)):

```go
type Engine interface {
    Detector
    ChunkerSelector
    Differ
    Previewer
    Metadata() *core.FileMetadata
}
```

### Sub-interfaces

| Interface | Method | Purpose |
|-----------|--------|---------|
| `Detector` | `Name() string` | Engine identifier (e.g. `"text"`, `"image"`) |
| `Detector` | `DetectByMagic(header []byte) bool` | **Layer 1** — magic bytes (strongest signal) |
| `Detector` | `DetectByExtension(path string) bool` | **Layer 2** — file extension |
| `Detector` | `DetectByHeuristic(path string, header []byte) bool` | **Layer 3** — content sniffing (weakest, fallback) |
| `ChunkerSelector` | `ChunkerFor(fileSize int64, cfg *core.CoreConfig) chunker.Chunker` | Returns a chunker for this file size |
| `Differ` | `Diff(oldPath, oldReader, newPath, newReader) (string, error)` | Streaming content diff |
| `Previewer` | `Preview(header, size, reader, maxLines) (string, error)` | Short human-readable preview |
| `Engine` | `Metadata() *core.FileMetadata` | Self-describing MIME type and metadata |

## Detection Order

Engines are registered in [`init.go`](../internal/filetype/init.go) in this
order:

1. **text** — UTF-8 text files
2. **image** — PNG, JPEG, GIF, WebP
3. **video** — MP4, WebM
4. **binary** — fallback (matches everything via `DetectByHeuristic → true`)

Detection runs three layers sequentially across all registered engines:

```
Layer 1: magic bytes  →  Layer 2: extension  →  Layer 3: heuristic
```

The first engine to return `true` at any layer wins. Binary's
`DetectByHeuristic` always returns `true`, so it acts as the catch-all
fallback — `DetectEngine` never returns nil when binary is registered.

## File Layout

Each engine lives in its own sub-package under
`internal/filetype/<name>/` and follows a **4-file convention**:

```
internal/filetype/<name>/
├── engine.go       # Engine struct + Detector methods + constructor
├── chunker.go      # ChunkerFor implementation
├── differ.go       # Diff implementation
├── preview.go      # Preview implementation
├── metadata.go     # Metadata implementation
└── engine_test.go  # Detection + behavior tests
```

## Adding a New Engine

### Step 1: Create the package

Create `internal/filetype/<name>/` with the 5 files above.

### Step 2: Implement the Engine struct

```go
// engine.go
package <name>

type FooEngine struct{}

func NewEngine() *FooEngine { return &FooEngine{} }

func (e *FooEngine) Name() string                         { return "foo" }
func (e *FooEngine) DetectByMagic(header []byte) bool     { /* check magic bytes */ }
func (e *FooEngine) DetectByExtension(path string) bool   { /* check .foo extension */ }
func (e *FooEngine) DetectByHeuristic(path string, header []byte) bool { /* sniff */ }
```

### Step 3: Implement ChunkerFor

Choose a chunking strategy appropriate for the file type. For most binary
formats, delegate to the shared size-tiered FastCDC selector:

```go
// chunker.go
func (e *FooEngine) ChunkerFor(fileSize int64, cfg *core.CoreConfig) chunker.Chunker {
    return chunker.DefaultSelector{}.ChunkerFor(fileSize, cfg)
}
```

For text-like formats with custom thresholds, implement a custom selector
(see `text/chunker.go` for an example).

### Step 4: Implement Diff

For text-based formats, produce a unified diff. For binary formats, return
a simple "binary files differ" message:

```go
// differ.go
func (e *FooEngine) Diff(oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error) {
    // Streaming — do not read entire file into memory unless necessary.
}
```

### Step 5: Implement Preview

```go
// preview.go
func (e *FooEngine) Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error) {
    // For binary formats: use header only, do NOT read from reader.
    // For text formats: read up to maxLines from reader.
}
```

### Step 6: Implement Metadata

```go
// metadata.go
func (e *FooEngine) Metadata() *core.FileMetadata {
    return &core.FileMetadata{MIMEType: "application/foo"}
}
```

### Step 7: Register the engine

Add the import and registration call in
[`internal/filetype/init.go`](../internal/filetype/init.go):

```go
func init() {
    Register(text.NewEngine())
    Register(image.NewEngine())
    Register(video.NewEngine())
    Register(foo.NewEngine())    // ← add before binary
    Register(binary.NewEngine()) // must be last (fallback)
}
```

**Registration order matters**: engines are queried in registration order at
each detection layer. Place specialized engines before `binary` (the
catch-all fallback).

### Step 8: Write tests

Create `engine_test.go` covering:
- Magic byte detection (positive and negative cases)
- Extension detection
- Heuristic detection
- Chunker selection at different file sizes
- Diff output format
- Preview output

## Design Principles

1. **Engines are self-describing**: porcelain never switches on
   `engine.Name()`. All type-specific behavior (including metadata) lives
   in the engine itself.

2. **Streaming over buffering**: `Diff` and `Preview` receive `io.Reader`,
   not `[]byte`. Engines that only need the header (image, video, binary)
   must not read from the reader — this keeps memory constant for large
   files.

3. **Chunker autonomy**: each engine decides its own chunking strategy via
   `ChunkerFor(fileSize, cfg)`. The binary selector is size-tiered (5
   bands); text has its own 2-band scheme.

4. **Detection layering**: magic bytes > extension > heuristic. Stronger
   signals are queried first so registration order does not affect
   correctness (only tie-breaking at the same layer).
