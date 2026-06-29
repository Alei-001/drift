package filetype

import (
	"github.com/your-org/drift/chunker"
)

// Engine combines all file-type-specific capabilities.
type Engine interface {
	Detector
	ChunkerSelector
	Differ
	Previewer
}

// Detector checks if an engine can handle a file using layered detection.
// Layers are queried in order of decreasing reliability: magic bytes first,
// then extension, then heuristic. This ensures precise signatures always
// take precedence over weak heuristics.
type Detector interface {
	Name() string
	// DetectByMagic checks file header signatures (strongest evidence).
	// Returns true only if the header matches a known, unambiguous magic
	// byte sequence for this engine. Must NOT depend on path/extension.
	// Engines without magic signatures (text, binary) return false.
	DetectByMagic(header []byte) bool
	// DetectByExtension checks file name/extension (medium evidence).
	// Returns true if the path's extension or basename is a known type
	// for this engine. Must NOT inspect header content.
	DetectByExtension(path string) bool
	// DetectByHeuristic is the last-resort content sniffing (weakest).
	// Only called when no engine matched by magic or extension.
	// Used for extensionless or unknown-extension files.
	DetectByHeuristic(path string, header []byte) bool
}

// ChunkerSelector returns a chunker appropriate for a file of the given size.
// Returning nil signals "whole-file single chunk": the caller reads the entire
// file and builds one chunk whose hash is BLAKE3(content).
//
// Each engine decides its own chunking strategy based on file size, keeping
// the policy co-located with the type that understands it. This lets future
// engines (e.g. PSD) return structure-aware chunkers without touching the
// snapshot layer.
type ChunkerSelector interface {
	ChunkerFor(fileSize int64) chunker.Chunker
}

// Differ produces diff between two file contents.
type Differ interface {
	Diff(oldPath string, oldData []byte, newPath string, newData []byte) (string, error)
}

// Previewer generates a text preview of file content.
type Previewer interface {
	Preview(data []byte, maxLines int) string
}
