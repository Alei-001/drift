package porcelain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/filetype/binary"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/storage/refname"
	"github.com/your-org/drift/util/fsutil"
	"github.com/your-org/drift/util/pathutil"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

func chunkFile(path string, r io.Reader, engine filetype.Engine, fileSize int64, cfg *core.CoreConfig) ([]*core.Chunk, error) {
	c := engine.ChunkerFor(fileSize, cfg)
	if fileSize == 0 {
		c = nil
	}
	if c == nil {
		// Reject large files before reading them into memory. The
		// nil-chunker path reads the whole file as a single chunk, so
		// a 500 MB video would OOM. 64 KB matches TextEngine's
		// whole-file threshold.
		if fileSize > 64*1024 {
			return nil, fmt.Errorf("file %s too large (%d bytes) for whole-file chunking without chunker", path, fileSize)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		// core.Chunk.Size is uint32; reject files whose single-chunk
		// representation would overflow it. In practice this path is
		// only reached for small text files (< 64KB), but guard
		// defensively in case a future engine returns nil for a
		// large file.
		if uint64(len(data)) > math.MaxUint32 {
			return nil, fmt.Errorf("file too large for single-chunk storage (%d bytes)", len(data))
		}
		sum := blake3.Sum256(data)
		var hash core.Hash
		copy(hash[:], sum[:])
		chunk := &core.Chunk{
			Hash:  hash,
			Size:  uint32(len(data)),
			Data:  data,
			Flags: core.ChunkFlagNone,
		}
		return []*core.Chunk{chunk}, nil
	}
	return c.Chunk(r)
}

func computeFileHashFromChunks(chunks []*core.Chunk) core.Hash {
	fileHasher := blake3.New()
	for _, c := range chunks {
		fileHasher.Write(c.Hash[:])
	}
	var fileHash core.Hash
	copy(fileHash[:], fileHasher.Sum(nil))
	return fileHash
}

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
			engine = &binary.BinaryEngine{}
		}

		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return nil, fmt.Errorf("seek %s: %w", f.path, err)
		}

		chunks, err := chunkFile(f.path, file, engine, f.info.Size(), cfg)
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
		if m := engineMetadata(engine); m != nil {
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
	if err := store.SetIndex(ctx, newIndex); err != nil {
		return nil, fmt.Errorf("update index: %w", err)
	}

	branchRef := &core.Reference{
		Name:   symRef,
		Type:   core.RefTypeBranch,
		Target: snap.ID.Hash,
	}
	if err := store.SetRef(ctx, symRef, branchRef); err != nil {
		return nil, fmt.Errorf("update branch: %w", err)
	}

	return snap, nil
}

func engineMetadata(engine filetype.Engine) *core.FileMetadata {
	name := engine.Name()
	switch name {
	case "text":
		return &core.FileMetadata{MIMEType: "text/plain"}
	case "binary", "image", "video":
		return &core.FileMetadata{MIMEType: "application/octet-stream"}
	default:
		return nil
	}
}

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
		engine = &binary.BinaryEngine{}
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

	chunks, err := chunkFile(filePath, file, engine, info.Size(), cfg)
	if err != nil {
		return core.Hash{}, fmt.Errorf("chunk file %s: %w", filePath, err)
	}

	return computeFileHashFromChunks(chunks), nil
}

// SaveTag creates a tag ref pointing at snapshotID. The existence check and
// the ref write are guarded by the workspace lock so that two concurrent
// SaveTag calls for the same name cannot both pass the check and the second
// cannot silently overwrite the first (TOCTOU).
func SaveTag(ctx context.Context, store storage.Storer, cwd string, name string, snapshotID core.Hash) error {
	if snapshotID.IsZero() {
		return fmt.Errorf("cannot create tag pointing to zero hash")
	}
	if name == "" {
		return fmt.Errorf("tag name is required")
	}
	if err := refname.Validate("tags/" + name); err != nil {
		return fmt.Errorf("invalid tag name: %w", err)
	}

	if err := AcquireWorkspaceLock(cwd); err != nil {
		return fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(cwd)

	refName := "tags/" + name
	existing, err := store.GetRef(ctx, refName)
	if err == nil && existing != nil {
		return fmt.Errorf("tag '%s' already exists: %w", name, ErrTagAlreadyExists)
	} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("check tag existence: %w", err)
	}

	ref := &core.Reference{
		Type:   core.RefTypeTag,
		Name:   refName,
		Target: snapshotID,
	}
	if err := store.SetRef(ctx, refName, ref); err != nil {
		return fmt.Errorf("set tag ref: %w", err)
	}
	return nil
}
