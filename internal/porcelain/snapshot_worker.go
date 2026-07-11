package porcelain

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/filetype"
	"github.com/Alei-001/drift/internal/storage"
	"github.com/schollz/progressbar/v3"
)

// workspaceFile pairs a filesystem path with its os.FileInfo, collected
// during the workspace walk in createSnapshotInLock.
type workspaceFile struct {
	path string
	info os.FileInfo
}

// fileTask represents a changed file that needs chunking and storage.
type fileTask struct {
	wf      workspaceFile
	relPath string
}

// processFileTask chunks a single file, stores new chunks, and returns the
// resulting FileEntry. It is designed to be called concurrently from
// multiple goroutines: each task opens its own file handle, and chunk
// storage is content-addressed (PutChunk writes to distinct paths keyed by
// hash, so concurrent writes never collide).
func processFileTask(ctx context.Context, store storage.Storer, task fileTask) (core.FileEntry, error) {
	file, err := os.Open(task.wf.path)
	if err != nil {
		return core.FileEntry{}, fmt.Errorf("open file %s: %w", task.wf.path, err)
	}
	defer file.Close()

	header, err := io.ReadAll(io.LimitReader(file, core.HeaderPeekSize))
	if err != nil {
		return core.FileEntry{}, fmt.Errorf("read header %s: %w", task.wf.path, err)
	}
	engine := filetype.DetectEngine(task.relPath, header)
	if engine == nil {
		return core.FileEntry{}, fmt.Errorf("no engine detected for %s", task.relPath)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return core.FileEntry{}, fmt.Errorf("seek %s: %w", task.wf.path, err)
	}

	chunks, err := chunkFile(ctx, task.wf.path, file, engine, task.wf.info.Size())
	if err != nil {
		return core.FileEntry{}, fmt.Errorf("chunk file %s: %w", task.wf.path, err)
	}

	var chunkHashes []core.Hash
	for _, c := range chunks {
		if err := ctx.Err(); err != nil {
			return core.FileEntry{}, err
		}
		has, err := store.HasChunk(ctx, c.Hash)
		if err != nil {
			return core.FileEntry{}, fmt.Errorf("check chunk existence %s: %w", c.Hash.String(), err)
		}
		if !has {
			if err := store.PutChunk(ctx, c); err != nil {
				return core.FileEntry{}, fmt.Errorf("store chunk %s: %w", c.Hash.String(), err)
			}
		}
		chunkHashes = append(chunkHashes, c.Hash)
	}

	fileHash := computeFileHashFromChunks(chunks)

	var metadata *core.FileMetadata
	if m := engine.Metadata(); m != nil {
		metadata = m
	}

	return core.FileEntry{
		Path:     task.relPath,
		Mode:     core.FileMode(task.wf.info.Mode()),
		Size:     task.wf.info.Size(),
		ModTime:  task.wf.info.ModTime().UnixNano(),
		Chunks:   chunkHashes,
		Hash:     fileHash,
		Metadata: metadata,
	}, nil
}

// chunkFilesConcurrent processes a list of file tasks using a worker pool
// sized to runtime.NumCPU(). Results are returned in a map keyed by the
// file's relative path. If any task fails, the first error is returned
// (remaining workers finish their current task but no new tasks are sent).
// The progress bar (if non-nil) is incremented after each file completes.
func chunkFilesConcurrent(ctx context.Context, store storage.Storer, tasks []fileTask, bar *progressbar.ProgressBar) (map[string]core.FileEntry, error) {
	results := make(map[string]core.FileEntry, len(tasks))
	if len(tasks) == 0 {
		return results, nil
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > len(tasks) {
		numWorkers = len(tasks)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, numWorkers)
	var wg sync.WaitGroup

	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			firstErr = err
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(t fileTask) {
			defer wg.Done()
			defer func() { <-sem }()

			entry, err := processFileTask(ctx, store, t)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			results[t.relPath] = entry
			mu.Unlock()

			if bar != nil {
				bar.Add(1) //nolint:errcheck
			}
		}(task)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}
