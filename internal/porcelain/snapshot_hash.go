package porcelain

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/filetype"
)

// ComputeFileHash returns the BLAKE3 file hash for filePath by chunking it
// with the detected engine and hashing the concatenation of chunk hashes.
// The hash is independent of chunk data layout and matches the hash
// CreateSnapshot would produce for the same file.
func ComputeFileHash(filePath string) (core.Hash, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return core.Hash{}, fmt.Errorf("open file %s: %w", filePath, err)
	}
	defer file.Close()

	header, err := io.ReadAll(io.LimitReader(file, core.HeaderPeekSize))
	if err != nil {
		return core.Hash{}, fmt.Errorf("read header %s: %w", filePath, err)
	}
	engine := filetype.DetectEngine(filePath, header)
	if engine == nil {
		return core.Hash{}, fmt.Errorf("no engine detected for %s", filePath)
	}

	info, err := file.Stat()
	if err != nil {
		return core.Hash{}, fmt.Errorf("stat file %s: %w", filePath, err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return core.Hash{}, fmt.Errorf("seek %s: %w", filePath, err)
	}

	chunks, err := chunkFile(context.Background(), filePath, file, engine, info.Size())
	if err != nil {
		return core.Hash{}, fmt.Errorf("chunk file %s: %w", filePath, err)
	}

	return computeFileHashFromChunks(chunks), nil
}
