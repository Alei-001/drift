package porcelain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/your-org/drift/internal/core"
	"github.com/your-org/drift/internal/filetype"
	"github.com/your-org/drift/internal/storage"
	"github.com/your-org/drift/internal/util/fsutil"
	"github.com/your-org/drift/internal/util/pathutil"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// CreateSnapshot scans workDir, chunks new or modified files, stores them,
// and writes a new snapshot on the current branch (HEAD's symbolic target,
// defaulting to heads/main). The workspace lock is acquired for the duration
// of the save. message must be non-empty; author defaults to "drift" when
// empty; tags may be nil or empty; cfg may be nil (core.DefaultConfig is
// used). Returns ErrNothingToSave when the workspace matches the index.
func CreateSnapshot(ctx context.Context, store storage.Storer, workDir string, message string, author string, tags []string, cfg *core.CoreConfig) (*core.Snapshot, error) {
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)
	return createSnapshotInLock(ctx, store, workDir, message, author, tags, cfg)
}

func createSnapshotInLock(ctx context.Context, store storage.Storer, workDir string, message string, author string, tags []string, cfg *core.CoreConfig) (*core.Snapshot, error) {
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if author == "" {
		author = "drift"
	}
	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}

	var headHash core.Hash
	headRef, err := store.GetRef(ctx, "HEAD")
	if err == nil {
		headHash = headRef.Target
	} else if !errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("read HEAD reference: %w", err)
	}

	oldIndex, err := store.GetIndex(ctx)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, fmt.Errorf("read index: %w", err)
		}
		oldIndex = &core.Index{}
	}

	type fileInfo struct {
		path string
		info os.FileInfo
	}
	var workspaceFiles []fileInfo
	err = fsutil.Walk(workDir, cfg.IgnoreFile, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		workspaceFiles = append(workspaceFiles, fileInfo{path: path, info: info})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}

	oldIndexMap := make(map[string]*core.IndexEntry)
	for i := range oldIndex.Entries {
		oldIndexMap[oldIndex.Entries[i].Path] = &oldIndex.Entries[i]
	}

	var fileEntries []core.FileEntry
	var totalSize int64
	changed := false

	for _, f := range workspaceFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		relPath, err := pathutil.Rel(workDir, f.path)
		if err != nil {
			return nil, fmt.Errorf("relative path for %s: %w", f.path, err)
		}

		if oldEntry, ok := oldIndexMap[relPath]; ok &&
			oldEntry.Size == f.info.Size() &&
			oldEntry.ModTime == f.info.ModTime().UnixNano() {
			entry := core.FileEntry{
				Path:    relPath,
				Mode:    core.FileMode(f.info.Mode()),
				Size:    f.info.Size(),
				ModTime: f.info.ModTime().UnixNano(),
				Chunks:  oldEntry.Chunks,
				Hash:    oldEntry.Hash,
			}
			fileEntries = append(fileEntries, entry)
			totalSize += f.info.Size()
			continue
		}

		file, err := os.Open(f.path)
		if err != nil {
			return nil, fmt.Errorf("open file %s: %w", f.path, err)
		}

		header, err := io.ReadAll(io.LimitReader(file, core.HeaderPeekSize))
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("read header %s: %w", f.path, err)
		}
		engine := filetype.DetectEngine(relPath, header)
		if engine == nil {
			file.Close()
			return nil, fmt.Errorf("no engine detected for %s", relPath)
		}

		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return nil, fmt.Errorf("seek %s: %w", f.path, err)
		}

		chunks, err := chunkFile(ctx, f.path, file, engine, f.info.Size(), cfg)
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("chunk file %s: %w", f.path, err)
		}

		var chunkHashes []core.Hash
		for _, c := range chunks {
			has, err := store.HasChunk(ctx, c.Hash)
			if err != nil {
				return nil, fmt.Errorf("check chunk existence %s: %w", c.Hash.String(), err)
			}
			if !has {
				if err := store.PutChunk(ctx, c); err != nil {
					return nil, fmt.Errorf("store chunk %s: %w", c.Hash.String(), err)
				}
			}
			chunkHashes = append(chunkHashes, c.Hash)
		}

		fileHash := computeFileHashFromChunks(chunks)

		var metadata *core.FileMetadata
		if m := engine.Metadata(); m != nil {
			metadata = m
		}

		entry := core.FileEntry{
			Path:     relPath,
			Mode:     core.FileMode(f.info.Mode()),
			Size:     f.info.Size(),
			ModTime:  f.info.ModTime().UnixNano(),
			Chunks:   chunkHashes,
			Hash:     fileHash,
			Metadata: metadata,
		}
		fileEntries = append(fileEntries, entry)
		totalSize += f.info.Size()

		if oldEntry, ok := oldIndexMap[relPath]; !ok {
			changed = true
		} else if oldEntry.Size != f.info.Size() || oldEntry.ModTime != f.info.ModTime().UnixNano() {
			changed = true
		} else if oldEntry.Hash != fileHash {
			changed = true
		}
	}

	if !changed && len(oldIndexMap) != len(workspaceFiles) {
		changed = true
	}

	if !changed {
		return nil, ErrNothingToSave
	}

	var prevID *core.SnapshotID
	if !headHash.IsZero() {
		prevID = &core.SnapshotID{Hash: headHash}
	}

	snap := &core.Snapshot{
		PrevID:    prevID,
		Message:   message,
		Author:    author,
		Timestamp: time.Now().Unix(),
		Files:     fileEntries,
		TotalSize: totalSize,
		Tags:      tags,
	}

	snapProto := core.SnapshotToProto(snap, false)
	marshaled, err := proto.Marshal(snapProto)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	var hash [32]byte = blake3.Sum256(marshaled)
	snap.ID = core.SnapshotID{Hash: core.Hash(hash)}

	if err := store.PutSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	symRef := "heads/main"
	if headRef != nil && headRef.SymRef != "" {
		symRef = headRef.SymRef
	}

	newIndex := &core.Index{
		UpdatedAt: time.Now().Unix(),
	}
	for _, entry := range fileEntries {
		newIndex.Entries = append(newIndex.Entries, core.IndexEntry{
			Path:    entry.Path,
			Size:    entry.Size,
			ModTime: entry.ModTime,
			Chunks:  entry.Chunks,
			Hash:    entry.Hash,
		})
	}
	branchRef := &core.Reference{
		Name:   symRef,
		Type:   core.RefTypeBranch,
		Target: snap.ID.Hash,
	}
	if err := store.SetRef(ctx, symRef, branchRef); err != nil {
		return nil, fmt.Errorf("update branch: %w", err)
	}

	if err := store.SetIndex(ctx, newIndex); err != nil {
		return nil, fmt.Errorf("update index: %w", err)
	}

	return snap, nil
}

// ComputeFileHash returns the BLAKE3 file hash for filePath by chunking it
// with the detected engine and hashing the concatenation of chunk hashes.
// cfg may be nil (core.DefaultConfig is used). The hash is independent of
// chunk data layout and matches the hash CreateSnapshot would produce for
// the same file.
func ComputeFileHash(filePath string, cfg *core.CoreConfig) (core.Hash, error) {
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

	if cfg == nil {
		cfg = &core.DefaultConfig().Core
	}

	chunks, err := chunkFile(context.Background(), filePath, file, engine, info.Size(), cfg)
	if err != nil {
		return core.Hash{}, fmt.Errorf("chunk file %s: %w", filePath, err)
	}

	return computeFileHashFromChunks(chunks), nil
}
