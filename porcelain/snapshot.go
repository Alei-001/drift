package porcelain

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/your-org/drift/core"
	"github.com/your-org/drift/filetype"
	"github.com/your-org/drift/storage"
	"github.com/your-org/drift/util/fsutil"
	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
)

// CreateSnapshot creates a snapshot of the current workspace state.
// If message is empty, the caller should open an editor to get one.
func CreateSnapshot(store storage.Storer, workDir string, message string, author string) (*core.Snapshot, error) {
	if message == "" {
		return nil, fmt.Errorf("message is required")
	}
	if author == "" {
		author = "drift"
	}

	// Get HEAD reference to find current HEAD hash
	var headHash core.Hash
	headRef, err := store.GetRef("HEAD")
	if err == nil {
		headHash = headRef.Target
	}

	// Get current index
	oldIndex, err := store.GetIndex()
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
		relPath, err := filepath.Rel(workDir, f.path)
		if err != nil {
			relPath = f.path
		}
		relPath = filepath.ToSlash(relPath)

		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", f.path, err)
		}

		// Detect engine for chunking
		header := data
		if len(header) > 512 {
			header = header[:512]
		}
		engine := filetype.DetectEngine(relPath, header)

		// Chunk the file
		chunks, err := engine.Chunk(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("chunk file %s: %w", f.path, err)
		}

		// Store chunks and collect hashes
		var chunkHashes []core.Hash
		for _, c := range chunks {
			if !store.HasChunk(c.Hash) {
				if err := store.PutChunk(c); err != nil {
					return nil, fmt.Errorf("store chunk: %w", err)
				}
			}
			chunkHashes = append(chunkHashes, c.Hash)
		}

		entry := core.FileEntry{
			Path:    relPath,
			Mode:    core.FileModeRegular,
			Size:    f.info.Size(),
			ModTime: f.info.ModTime().Unix(),
			Chunks:  chunkHashes,
		}
		fileEntries = append(fileEntries, entry)
		totalSize += f.info.Size()

		// Detect changes: added or modified
		if oldEntry, ok := oldIndexMap[relPath]; !ok {
			changed = true
		} else if oldEntry.Size != f.info.Size() || oldEntry.ModTime != f.info.ModTime().Unix() {
			changed = true
		}
	}

	// Detect deletions
	if !changed && len(oldIndexMap) != len(workspaceFiles) {
		changed = true
	}

	if !changed {
		return nil, fmt.Errorf("nothing to save")
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
	}

	// Compute snapshot hash: marshal to protobuf, hash with BLAKE3
	snapProto := snapshotToProto(snap)
	marshaled, err := proto.Marshal(snapProto)
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}
	var hash [32]byte = blake3.Sum256(marshaled)
	snap.ID = core.SnapshotID{Hash: core.Hash(hash)}

	// Save snapshot
	if err := store.PutSnapshot(snap); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}

	// Update HEAD reference
	newHeadRef := &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: snap.ID.Hash,
	}
	if err := store.SetRef("HEAD", newHeadRef); err != nil {
		return nil, fmt.Errorf("update HEAD: %w", err)
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
		})
	}
	if err := store.SetIndex(newIndex); err != nil {
		return nil, fmt.Errorf("update index: %w", err)
	}

	return snap, nil
}

// snapshotToProto converts a core.Snapshot to core.SnapshotProto for hashing.
// The ID hash is set to nil so the hash can be computed over the rest of the fields.
func snapshotToProto(s *core.Snapshot) *core.SnapshotProto {
	sp := &core.SnapshotProto{
		Message:   s.Message,
		Author:    s.Author,
		Timestamp: s.Timestamp,
		Tags:      s.Tags,
		TotalSize: s.TotalSize,
	}

	if s.PrevID != nil {
		prevHash := make([]byte, 32)
		copy(prevHash, s.PrevID.Hash[:])
		sp.PrevIdHash = prevHash
	}

	for _, f := range s.Files {
		fp := &core.FileEntryProto{
			Path:    f.Path,
			Mode:    uint32(f.Mode),
			Size:    f.Size,
			ModTime: f.ModTime,
		}

		for _, h := range f.Chunks {
			ch := make([]byte, 32)
			copy(ch, h[:])
			fp.ChunkHashes = append(fp.ChunkHashes, ch)
		}

		if f.Metadata != nil && f.Metadata.MimeType != "" {
			mt := f.Metadata.MimeType
			fp.MimeType = &mt
		}

		sp.Files = append(sp.Files, fp)
	}

	return sp
}
