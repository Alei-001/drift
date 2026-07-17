package snapshot

import (
	"context"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/engine"
	"github.com/Alei-001/drift/internal/store"
	"github.com/Alei-001/drift/internal/store/stream"
	"github.com/Alei-001/drift/internal/util/format"
)

// DetectFileTypeLabel returns a human-readable label for the file type of
// the given snapshot file entry, by reading its chunks from storage and
// detecting the filetype engine. Image files include parsed dimensions
// when available. It returns "binary" when the chunks cannot be read or
// no engine matches.
func DetectFileTypeLabel(ctx context.Context, st *store.StoreSet, entry *core.FileEntry) string {
	chunkR := stream.NewChunkReader(ctx, st.Chunks, entry.Chunks)
	header, _, err := stream.PeekHeader(chunkR, core.HeaderPeekSize)
	if err != nil {
		return "binary"
	}
	engine := engine.DetectEngine(entry.Path, header)
	if engine == nil {
		return "binary"
	}
	switch engine.Name() {
	case "text":
		return "text"
	case "image":
		if dims := format.ImageDimensions(header); dims != "" {
			return "image (" + dims + ")"
		}
		return "image"
	default:
		return engine.Name()
	}
}
