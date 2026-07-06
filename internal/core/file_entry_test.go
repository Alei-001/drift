package core

import "testing"

func TestFileEntry_Fields(t *testing.T) {
	chunks := []Hash{{0x11}, {0x22}}
	fe := FileEntry{
		Path:     "src/main.go",
		Mode:     FileModeRegular,
		Size:     1024,
		ModTime:  1700000000,
		Chunks:   chunks,
		Hash:     Hash{0xab},
		Metadata: &FileMetadata{MIMEType: "text/plain"},
	}
	if fe.Path != "src/main.go" {
		t.Errorf("Path: got %q, want %q", fe.Path, "src/main.go")
	}
	if !fe.Mode.IsRegular() {
		t.Errorf("Mode should be regular, got %v", fe.Mode)
	}
	if fe.Size != 1024 {
		t.Errorf("Size: got %d, want 1024", fe.Size)
	}
	if fe.ModTime != 1700000000 {
		t.Errorf("ModTime: got %d, want 1700000000", fe.ModTime)
	}
	if len(fe.Chunks) != 2 {
		t.Errorf("Chunks len: got %d, want 2", len(fe.Chunks))
	}
	if fe.Chunks[0] != (Hash{0x11}) {
		t.Errorf("Chunks[0]: got %v, want %v", fe.Chunks[0], Hash{0x11})
	}
	if fe.Hash != (Hash{0xab}) {
		t.Errorf("Hash: got %v, want %v", fe.Hash, Hash{0xab})
	}
	if fe.Metadata == nil || fe.Metadata.MIMEType != "text/plain" {
		t.Errorf("Metadata.MIMEType: got %v, want %q", fe.Metadata, "text/plain")
	}
}

func TestFileEntry_Empty(t *testing.T) {
	fe := FileEntry{}
	if fe.Path != "" {
		t.Errorf("Path: got %q, want empty", fe.Path)
	}
	if fe.Size != 0 {
		t.Errorf("Size: got %d, want 0", fe.Size)
	}
	if fe.Chunks != nil {
		t.Error("expected nil Chunks for empty entry")
	}
	if fe.Metadata != nil {
		t.Error("expected nil Metadata for empty entry")
	}
}

func TestFileMetadata_Extra(t *testing.T) {
	m := &FileMetadata{
		MIMEType: "image/png",
		Extra:    map[string]string{"width": "100", "height": "200"},
	}
	if m.MIMEType != "image/png" {
		t.Errorf("MIMEType: got %q, want %q", m.MIMEType, "image/png")
	}
	if len(m.Extra) != 2 {
		t.Errorf("Extra len: got %d, want 2", len(m.Extra))
	}
	if m.Extra["width"] != "100" {
		t.Errorf("Extra[width]: got %q, want %q", m.Extra["width"], "100")
	}
	if m.Extra["height"] != "200" {
		t.Errorf("Extra[height]: got %q, want %q", m.Extra["height"], "200")
	}
}

func TestFileMetadata_Empty(t *testing.T) {
	m := &FileMetadata{}
	if m.MIMEType != "" {
		t.Errorf("MIMEType: got %q, want empty", m.MIMEType)
	}
	if m.Extra != nil {
		t.Error("expected nil Extra for empty metadata")
	}
}

func TestIndexEntry_Fields(t *testing.T) {
	chunks := []Hash{{0x11}}
	ie := IndexEntry{
		Path:    "file.txt",
		Hash:    Hash{0xab},
		Size:    100,
		ModTime: 42,
		Chunks:  chunks,
	}
	if ie.Path != "file.txt" {
		t.Errorf("Path: got %q, want %q", ie.Path, "file.txt")
	}
	if ie.Hash != (Hash{0xab}) {
		t.Errorf("Hash: got %v, want %v", ie.Hash, Hash{0xab})
	}
	if ie.Size != 100 {
		t.Errorf("Size: got %d, want 100", ie.Size)
	}
	if ie.ModTime != 42 {
		t.Errorf("ModTime: got %d, want 42", ie.ModTime)
	}
	if len(ie.Chunks) != 1 || ie.Chunks[0] != (Hash{0x11}) {
		t.Errorf("Chunks: got %v, want [%v]", ie.Chunks, Hash{0x11})
	}
}

func TestIndex_Fields(t *testing.T) {
	idx := &Index{
		Entries:   []IndexEntry{{Path: "a.txt"}},
		UpdatedAt: 12345,
	}
	if len(idx.Entries) != 1 {
		t.Errorf("Entries len: got %d, want 1", len(idx.Entries))
	}
	if idx.UpdatedAt != 12345 {
		t.Errorf("UpdatedAt: got %d, want 12345", idx.UpdatedAt)
	}
}

func TestIndex_Empty(t *testing.T) {
	idx := &Index{}
	if len(idx.Entries) != 0 {
		t.Errorf("Entries len: got %d, want 0", len(idx.Entries))
	}
	if idx.UpdatedAt != 0 {
		t.Errorf("UpdatedAt: got %d, want 0", idx.UpdatedAt)
	}
}
