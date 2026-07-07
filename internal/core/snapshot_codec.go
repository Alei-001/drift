package core

// SnapshotToProto converts a Snapshot to its protobuf representation.
//
// withIDHash controls whether the IdHash field is populated:
//   - false: used when computing the snapshot ID hash (porcelain layer), where
//     the ID is not yet known. Omitting IdHash keeps the hash stable regardless
//     of the (still unassigned) ID.
//   - true: used when persisting a snapshot (storage layer), where the ID has
//     already been assigned.
//
// All byte slices are defensively copied so the returned proto does not alias
// the caller's memory; mutating the proto afterwards cannot corrupt the source
// Snapshot and vice versa.
func SnapshotToProto(s *Snapshot, withIDHash bool) *SnapshotProto {
	if s == nil {
		return nil
	}
	p := &SnapshotProto{
		Message:   s.Message,
		Author:    s.Author,
		Timestamp: s.Timestamp,
		Tags:      s.Tags,
		TotalSize: s.TotalSize,
	}
	if s.PrevID != nil {
		p.PrevIdHash = make([]byte, HashSize)
		copy(p.PrevIdHash, s.PrevID.Hash[:])
	}
	if withIDHash && s.ID != (SnapshotID{}) {
		p.IdHash = make([]byte, HashSize)
		copy(p.IdHash, s.ID.Hash[:])
	}
	p.Files = make([]*FileEntryProto, len(s.Files))
	for i := range s.Files {
		p.Files[i] = fileEntryToProto(&s.Files[i])
	}
	return p
}

// fileEntryToProto converts a FileEntry to its protobuf representation.
//
// The file_hash field (field 7) stores the file-level BLAKE3 hash so that
// snapshotFromProto can read it back directly instead of recomputing it from
// the chunk hashes. Writing it here (rather than in only one layer) keeps the
// porcelain and storage serializations identical apart from the IdHash field,
// so the snapshot ID hash stays stable across the two layers.
func fileEntryToProto(f *FileEntry) *FileEntryProto {
	fp := &FileEntryProto{
		Path:    f.Path,
		Mode:    uint32(f.Mode),
		Size:    f.Size,
		ModTime: f.ModTime,
	}
	if len(f.Chunks) > 0 {
		fp.ChunkHashes = make([][]byte, len(f.Chunks))
		for i, c := range f.Chunks {
			fp.ChunkHashes[i] = make([]byte, HashSize)
			copy(fp.ChunkHashes[i], c[:])
		}
	}
	if f.Metadata != nil {
		if f.Metadata.MIMEType != "" {
			mt := f.Metadata.MIMEType
			fp.MimeType = &mt
		}
		if len(f.Metadata.Extra) > 0 {
			fp.Extra = make(map[string]string, len(f.Metadata.Extra))
			for k, v := range f.Metadata.Extra {
				fp.Extra[k] = v
			}
		}
	}
	if f.Hash != (Hash{}) {
		fp.FileHash = make([]byte, HashSize)
		copy(fp.FileHash, f.Hash[:])
	}
	return fp
}

// SnapshotFromProto rebuilds a Snapshot from its protobuf wire form. This is
// the inverse of SnapshotToProto and is shared between the storage layer
// (GetSnapshot) and the remote sync layer (pull), so both decode persisted
// snapshots identically.
func SnapshotFromProto(p *SnapshotProto) *Snapshot {
	if p == nil {
		return nil
	}
	s := &Snapshot{
		ID:        SnapshotID{Hash: bytesToHash(p.IdHash)},
		Message:   p.Message,
		Author:    p.Author,
		Timestamp: p.Timestamp,
		Tags:      p.Tags,
		TotalSize: p.TotalSize,
	}
	if len(p.PrevIdHash) > 0 {
		prevID := SnapshotID{Hash: bytesToHash(p.PrevIdHash)}
		s.PrevID = &prevID
	}
	for _, fe := range p.Files {
		f := FileEntry{
			Path:    fe.Path,
			Mode:    FileMode(fe.Mode),
			Size:    fe.Size,
			ModTime: fe.ModTime,
		}
		for _, ch := range fe.ChunkHashes {
			f.Chunks = append(f.Chunks, bytesToHash(ch))
		}
		if len(fe.FileHash) == HashSize {
			copy(f.Hash[:], fe.FileHash)
		}
		if fe.MimeType != nil || len(fe.Extra) > 0 {
			f.Metadata = &FileMetadata{}
			if fe.MimeType != nil {
				f.Metadata.MIMEType = *fe.MimeType
			}
			if len(fe.Extra) > 0 {
				f.Metadata.Extra = make(map[string]string, len(fe.Extra))
				for k, v := range fe.Extra {
					f.Metadata.Extra[k] = v
				}
			}
		}
		s.Files = append(s.Files, f)
	}
	return s
}

// BytesToHash copies a byte slice into a Hash. The caller must ensure len(b)
// <= HashSize; extra bytes are truncated. Used when decoding protobuf
// IdHash/PrevIdHash/ChunkHashes fields.
func BytesToHash(b []byte) Hash {
	var h Hash
	copy(h[:], b)
	return h
}

// bytesToHash is an unexported alias for use within the core package.
func bytesToHash(b []byte) Hash { return BytesToHash(b) }
