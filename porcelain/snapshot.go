package porcelain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
	"github.com/your-org/drift/util/pathutil"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// chunkFile reads from r and chunks it using the strategy selected by the
// engine for the given file size. When the engine's ChunkerFor returns nil
// (whole-file single chunk), the entire content is read and returned as one
// chunk whose Hash is BLAKE3(content).
//
// Both CreateSnapshot and ComputeFileHash use this helper so that the chunk
// layout—and therefore the file hash—stays identical between them.
func chunkFile(r io.Reader, engine filetype.Engine, fileSize int64) ([]*core.Chunk, error) {
	c := engine.ChunkerFor(fileSize)
	// Empty files always go through the whole-file single-chunk path
	// regardless of engine, so that empty .txt and empty .bin produce
	// identical chunk layouts (and therefore identical file hashes).
	// Without this, TextEngine returns nil (whole-file) for size 0 while
	// BinaryEngine returns a FastCDC chunker whose Chunk() skips
	// zero-length data, yielding zero chunks and a different file hash.
	if fileSize == 0 {
		c = nil
	}
	if c == nil {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
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

// computeFileHashFromChunks computes the file-level hash by concatenating all
// chunk hashes and hashing with BLAKE3. This mirrors the original hashing
// logic used by CreateSnapshot and must be used by any code path that needs
// to match it (including the whole-file single-chunk path).
func computeFileHashFromChunks(chunks []*core.Chunk) core.Hash {
	fileHasher := blake3.New()
	for _, c := range chunks {
		fileHasher.Write(c.Hash[:])
	}
	var fileHash core.Hash
	copy(fileHash[:], fileHasher.Sum(nil))
	return fileHash
}

// CreateSnapshot creates a snapshot of the current workspace state.
// If message is empty, the caller should open an editor to get one.
// tags are optional labels attached to the snapshot (e.g. --tag "v1").
//
// It acquires the workspace lock for the duration of the snapshot so that
// concurrent workspace-modifying operations (switch, restore, daemon save,
// gc) cannot observe or mutate a half-written index. Callers that already
// hold the workspace lock must call createSnapshotInLock instead to avoid
// re-entrant locking.
func CreateSnapshot(ctx context.Context, store storage.Storer, workDir string, message string, author string, tags []string) (*core.Snapshot, error) {
	if err := AcquireWorkspaceLock(workDir); err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	defer ReleaseWorkspaceLock(workDir)
	return createSnapshotInLock(ctx, store, workDir, message, author, tags)
}

// createSnapshotInLock is the lock-free inner half of CreateSnapshot. It
// performs the actual workspace scan, chunking, and snapshot persistence.
// The caller MUST already hold the workspace lock — this function does not
// acquire it, to allow callers that already hold the lock (SwitchBranch,
// RestoreSnapshot, RunDaemonLoop) to compose without re-entrant locking.
func createSnapshotInLock(ctx context.Context, store storage.Storer, workDir string, message string, author string, tags []string) (*core.Snapshot, error) {
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if author == "" {
		author = "drift"
	}

	// Get HEAD reference to find current HEAD hash
	var headHash core.Hash
	headRef, err := store.GetRef(ctx, "HEAD")
	if err == nil {
		headHash = headRef.Target
	} else if !errors.Is(err, storage.ErrNotFound) {
		// HEAD exists but can't be read — this is a real error
		return nil, fmt.Errorf("read HEAD reference: %w", err)
	}

	// Get current index
	oldIndex, err := store.GetIndex(ctx)
	if err != nil {
		oldIndex = &core.Index{}
	}

	// Scan workspace
	type fileInfo struct {
		path string
		info os.FileInfo
	}
	var workspaceFiles []fileInfo
	err = fsutil.Walk(workDir, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		workspaceFiles = append(workspaceFiles, fileInfo{path: path, info: info})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}

	// Build index map for comparison
	oldIndexMap := make(map[string]*core.IndexEntry)
	for i := range oldIndex.Entries {
		oldIndexMap[oldIndex.Entries[i].Path] = &oldIndex.Entries[i]
	}

	// Process each file: chunk, store chunks, build FileEntry
	var fileEntries []core.FileEntry
	var totalSize int64
	changed := false

	for _, f := range workspaceFiles {
		// Convert absolute path to relative path (relative to workDir)
		relPath, err := pathutil.Rel(workDir, f.path)
		if err != nil {
			relPath = f.path
		}

		// Check if file is unchanged — skip chunking and reuse old chunks.
		// When size and mtime match, additionally verify the content hash so
		// that CreateSnapshot stays consistent with DetectChanges (which
		// always hashes). Without this, status could report a file modified
		// while save reports "nothing to save". ModTime uses UnixNano for
		// sub-second resolution so same-second edits are not missed.
		if oldEntry, ok := oldIndexMap[relPath]; ok &&
			oldEntry.Size == f.info.Size() &&
			oldEntry.ModTime == f.info.ModTime().UnixNano() {
			currentHash, hashErr := ComputeFileHash(f.path)
			if hashErr == nil && currentHash == oldEntry.Hash {
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
			// Hash mismatch or compute error — treat as modified and
			// fall through to the chunking path below.
		}

		// Open file for streaming reads
		file, err := os.Open(f.path)
		if err != nil {
			return nil, fmt.Errorf("open file %s: %w", f.path, err)
		}

		// Read up to first 512 bytes for engine detection
		header, err := io.ReadAll(io.LimitReader(file, 512))
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("read header %s: %w", f.path, err)
		}
		engine := filetype.DetectEngine(relPath, header)

		// Seek back to start for chunking
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return nil, fmt.Errorf("seek %s: %w", f.path, err)
		}

		// Chunk the file using the strategy selected by the engine for
		// this file size. chunkFile handles both the whole-file single-
		// chunk path (ChunkerFor returns nil) and the multi-chunk path.
		chunks, err := chunkFile(file, engine, f.info.Size())
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("chunk file %s: %w", f.path, err)
		}

		// Store chunks and collect hashes. The whole-file single-chunk
		// path also goes through PutChunk for dedup, so its chunk is
		// stored exactly like any other.
		var chunkHashes []core.Hash
		for _, c := range chunks {
			if !store.HasChunk(ctx, c.Hash) {
				if err := store.PutChunk(ctx, c); err != nil {
					return nil, fmt.Errorf("store chunk: %w", err)
				}
			}
			chunkHashes = append(chunkHashes, c.Hash)
		}

		// Compute file-level hash from chunk hashes. This stays identical
		// to the original logic so existing snapshots remain compatible;
		// for the single-chunk path it is BLAKE3(that chunk's hash).
		fileHash := computeFileHashFromChunks(chunks)

		entry := core.FileEntry{
			Path:    relPath,
			Mode:    core.FileMode(f.info.Mode()),
			Size:    f.info.Size(),
			ModTime: f.info.ModTime().UnixNano(),
			Chunks:  chunkHashes,
			Hash:    fileHash,
		}
		fileEntries = append(fileEntries, entry)
		totalSize += f.info.Size()

		// Detect changes: added or modified
		if oldEntry, ok := oldIndexMap[relPath]; !ok {
			changed = true
		} else if oldEntry.Size != f.info.Size() || oldEntry.ModTime != f.info.ModTime().UnixNano() {
			changed = true
		} else if oldEntry.Hash != fileHash {
			changed = true
		}
	}

	// Detect deletions
	if !changed && len(oldIndexMap) != len(workspaceFiles) {
		changed = true
	}

	if !changed {
		return nil, ErrNothingToSave
	}

	// Get previous snapshot if HEAD is not zero
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

	// Compute snapshot hash: marshal to protobuf, hash with BLAKE3.
	// withIDHash=false so the not-yet-assigned ID does not affect the hash.
	snapProto := core.SnapshotToProto(snap, false)
	marshaled, err := proto.Marshal(snapProto)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	var hash [32]byte = blake3.Sum256(marshaled)
	snap.ID = core.SnapshotID{Hash: core.Hash(hash)}

	// Save snapshot
	if err := store.PutSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	// Update the current branch ref (HEAD is a symref, e.g. heads/main)
	headRef, err = store.GetRef(ctx, "HEAD")
	symRef := "heads/main"
	if err == nil && headRef.SymRef != "" {
		symRef = headRef.SymRef
	}
	branchRef := &core.Reference{
		Name:   symRef,
		Type:   core.RefTypeBranch,
		Target: snap.ID.Hash,
	}
	if err := store.SetRef(ctx, symRef, branchRef); err != nil {
		return nil, fmt.Errorf("update branch: %w", err)
	}

	// Update index
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

	return snap, nil
}

// ComputeFileHash chunks a file with its detected engine and returns the
// BLAKE3 file hash computed from the chunk hashes. It mirrors the hashing
// performed during CreateSnapshot, so callers (e.g. status) can detect
// content changes even when size and modification time are unchanged.
// Chunks are not stored.
func ComputeFileHash(filePath string) (core.Hash, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return core.Hash{}, fmt.Errorf("open file %s: %w", filePath, err)
	}
	defer file.Close()

	header, err := io.ReadAll(io.LimitReader(file, 512))
	if err != nil {
		return core.Hash{}, fmt.Errorf("read header %s: %w", filePath, err)
	}
	engine := filetype.DetectEngine(filePath, header)

	// Stat the open file to get its size, which ChunkerFor needs to
	// pick the same strategy CreateSnapshot uses.
	info, err := file.Stat()
	if err != nil {
		return core.Hash{}, fmt.Errorf("stat file %s: %w", filePath, err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return core.Hash{}, fmt.Errorf("seek %s: %w", filePath, err)
	}

	// Use the same chunking strategy as CreateSnapshot so the resulting
	// file hash matches bit-for-bit.
	chunks, err := chunkFile(file, engine, info.Size())
	if err != nil {
		return core.Hash{}, fmt.Errorf("chunk file %s: %w", filePath, err)
	}

	return computeFileHashFromChunks(chunks), nil
}

// SaveTag creates a tag reference pointing to the given snapshot hash.
// It returns ErrTagAlreadyExists if the tag already exists.
func SaveTag(ctx context.Context, store storage.Storer, name string, snapshotID core.Hash) error {
	if name == "" {
		return fmt.Errorf("tag name is required")
	}

	refName := "tags/" + name
	if existing, err := store.GetRef(ctx, refName); err == nil && existing != nil {
		return fmt.Errorf("tag '%s' already exists: %w", name, ErrTagAlreadyExists)
	}

	ref := &core.Reference{
		Type:   core.RefTypeTag,
		Name:   name,
		Target: snapshotID,
	}
	if err := store.SetRef(ctx, refName, ref); err != nil {
		return fmt.Errorf("set tag ref: %w", err)
	}
	return nil
}
