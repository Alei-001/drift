package filetype

import (
	"io"

	"github.com/your-org/drift/internal/chunker"
	"github.com/your-org/drift/internal/core"
)

// Engine bundles the per-filetype capabilities a registered engine must
// provide: detection, chunker selection, diffing, preview, and metadata.
// Composing these in one interface lets the registry treat all engines
// uniformly and keeps each engine self-describing — adding a new filetype
// requires no edits outside its own package.
type Engine interface {
	Detector
	ChunkerSelector
	Differ
	Previewer
	// Metadata returns the file metadata describing files handled by this
	// engine (e.g. MIME type). Returning nil means "no metadata"; callers
	// must handle nil. Pushing this into the engine (rather than switching
	// on engine.Name() in porcelain) keeps the engine self-describing and
	// preserves the pluggable-engine contract: adding a new engine requires
	// no edits outside its own package.
	Metadata() *core.FileMetadata
}

// Detector identifies whether a file belongs to an engine via three layered
// signals, applied in priority order by the registry: magic bytes (strongest),
// file extension, and heuristic sniffing (weakest, used only as a fallback).
type Detector interface {
	Name() string
	DetectByMagic(header []byte) bool
	DetectByExtension(path string) bool
	DetectByHeuristic(path string, header []byte) bool
}

// ChunkerSelector chooses the chunking strategy for a file of the given size,
// honouring caller-supplied config when present. Returning nil signals that
// the caller should store the whole file as a single chunk.
type ChunkerSelector interface {
	ChunkerFor(fileSize int64, cfg *core.CoreConfig) chunker.Chunker
}

// Differ compares two files and returns a unified diff or summary.
// Implementations read content streaming from oldReader/newReader rather
// than receiving the whole file in memory.
type Differ interface {
	// Diff compares two files. oldPath/newPath are used for the diff header
	// and file type context; oldReader/newReader provide streaming content.
	Diff(oldPath string, oldReader io.Reader, newPath string, newReader io.Reader) (string, error)
}

// Previewer generates a short, human-readable preview of a file.
// header carries the leading bytes (for magic detection and dimension
// parsing), size is the total file size, and reader allows streaming
// access to the content. Engines that only need the header (image, video,
// binary) must not read from reader, keeping memory constant for large
// files.
type Previewer interface {
	// Preview returns a short summary. header is the file head (for magic
	// detection), size is the total file size, reader streams the content,
	// and maxLines bounds the number of lines for text previews.
	Preview(header []byte, size int64, reader io.Reader, maxLines int) (string, error)
}
