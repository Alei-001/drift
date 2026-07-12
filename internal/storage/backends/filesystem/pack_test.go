package filesystem

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/zeebo/blake3"
)

func makeChunk(data []byte) *core.Chunk {
	var hash core.Hash
	sum := blake3.Sum256(data)
	copy(hash[:], sum[:])
	return &core.Chunk{Hash: hash, Size: uint32(len(data)), Data: data}
}

func TestCreatePack_ReadFromPack(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	chunks := []*core.Chunk{
		makeChunk([]byte("pack test chunk one")),
		makeChunk([]byte("pack test chunk two")),
		makeChunk([]byte("pack test chunk three")),
	}

	hashes := make([]core.Hash, len(chunks))
	for i, ch := range chunks {
		if err := fs.PutChunk(context.Background(), ch); err != nil {
			t.Fatalf("PutChunk %d: %v", i, err)
		}
		hashes[i] = ch.Hash
	}

	if err := fs.createPack(context.Background(), hashes); err != nil {
		t.Fatalf("createPack: %v", err)
	}

	for _, h := range hashes {
		if err := fs.DeleteChunk(context.Background(), h); err != nil {
			t.Fatalf("DeleteChunk: %v", err)
		}
	}
	fs.chunkCache.Remove(hashes[0])

	for i, h := range hashes {
		got, err := fs.GetChunk(context.Background(), h)
		if err != nil {
			t.Fatalf("GetChunk %d from pack: %v", i, err)
		}
		if !bytes.Equal(got.Data, chunks[i].Data) {
			t.Fatalf("chunk %d data mismatch: got %q, want %q", i, got.Data, chunks[i].Data)
		}
	}
}

func TestCreatePack_ReadFromPack_Compressed(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	data := bytes.Repeat([]byte("compressible-pack-data-"), 1000)
	ch := makeChunk(data)
	if err := fs.PutChunk(context.Background(), ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	if err := fs.createPack(context.Background(), []core.Hash{ch.Hash}); err != nil {
		t.Fatalf("createPack: %v", err)
	}

	if err := fs.DeleteChunk(context.Background(), ch.Hash); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}
	fs.chunkCache.Remove(ch.Hash)

	got, err := fs.GetChunk(context.Background(), ch.Hash)
	if err != nil {
		t.Fatalf("GetChunk from pack: %v", err)
	}
	if !bytes.Equal(got.Data, data) {
		t.Fatalf("compressed chunk data mismatch: got len=%d, want len=%d", len(got.Data), len(data))
	}
}

func TestGetChunk_LooseBeforePack(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	looseData := []byte("loose version data")
	packData := []byte("pack version data -- different!")

	var hash core.Hash
	sum := blake3.Sum256(looseData)
	copy(hash[:], sum[:])

	looseChunk := &core.Chunk{Hash: hash, Size: uint32(len(looseData)), Data: looseData}
	if err := fs.PutChunk(context.Background(), looseChunk); err != nil {
		t.Fatalf("PutChunk loose: %v", err)
	}

	packChunk := &core.Chunk{Hash: hash, Size: uint32(len(packData)), Data: packData}
	if err := fs.createPack(context.Background(), []core.Hash{packChunk.Hash}); err != nil {
		t.Fatalf("createPack: %v", err)
	}

	got, err := fs.GetChunk(context.Background(), hash)
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	if !bytes.Equal(got.Data, looseData) {
		t.Fatalf("GetChunk should prefer loose data, got %q", got.Data)
	}
}

func TestListChunks_LooseAndPack(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	chunkA := makeChunk([]byte("chunk A for pack"))
	chunkB := makeChunk([]byte("chunk B loose only"))

	if err := fs.PutChunk(context.Background(), chunkA); err != nil {
		t.Fatalf("PutChunk A: %v", err)
	}
	if err := fs.PutChunk(context.Background(), chunkB); err != nil {
		t.Fatalf("PutChunk B: %v", err)
	}

	if err := fs.createPack(context.Background(), []core.Hash{chunkA.Hash}); err != nil {
		t.Fatalf("createPack: %v", err)
	}
	if err := fs.DeleteChunk(context.Background(), chunkA.Hash); err != nil {
		t.Fatalf("DeleteChunk A: %v", err)
	}

	hashes, err := fs.ListChunks(context.Background())
	if err != nil {
		t.Fatalf("ListChunks: %v", err)
	}

	foundA := false
	foundB := false
	for _, h := range hashes {
		if h == chunkA.Hash {
			foundA = true
		}
		if h == chunkB.Hash {
			foundB = true
		}
	}
	if !foundA {
		t.Fatal("chunkA should be listed (from pack)")
	}
	if !foundB {
		t.Fatal("chunkB should be listed (from loose)")
	}
}

func TestHasChunk_Pack(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	ch := makeChunk([]byte("chunk for pack existence check"))
	if err := fs.PutChunk(context.Background(), ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}
	if err := fs.createPack(context.Background(), []core.Hash{ch.Hash}); err != nil {
		t.Fatalf("createPack: %v", err)
	}
	if err := fs.DeleteChunk(context.Background(), ch.Hash); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}
	fs.chunkCache.Remove(ch.Hash)

	ok, err := fs.HasChunk(context.Background(), ch.Hash)
	if err != nil {
		t.Fatalf("HasChunk: %v", err)
	}
	if !ok {
		t.Fatal("HasChunk should find chunk in pack")
	}
}


func TestPackIndex_WriteRead(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	ch1 := makeChunk([]byte("index test chunk one"))
	ch2 := makeChunk([]byte("index test chunk two"))

	idx := &packIndex{
		name: "pack-00000001",
		entries: map[core.Hash]packEntry{
			ch1.Hash: {offset: 0, length: 42, flags: 0x00},
			ch2.Hash: {offset: 42, length: 43, flags: 0x01},
		},
	}

	if err := fs.writePackIndex("pack-00000001", idx); err != nil {
		t.Fatalf("writePackIndex: %v", err)
	}

	loaded, err := fs.loadPackIndex("pack-00000001")
	if err != nil {
		t.Fatalf("loadPackIndex: %v", err)
	}

	if loaded.name != "pack-00000001" {
		t.Errorf("name mismatch: got %q, want %q", loaded.name, "pack-00000001")
	}
	if len(loaded.entries) != 2 {
		t.Errorf("entry count mismatch: got %d, want 2", len(loaded.entries))
	}

	e1, ok := loaded.entries[ch1.Hash]
	if !ok {
		t.Fatal("chunk1 hash not found in loaded index")
	}
	if e1.offset != 0 || e1.length != 42 || e1.flags != 0x00 {
		t.Errorf("chunk1 entry mismatch: offset=%d length=%d flags=%d", e1.offset, e1.length, e1.flags)
	}

	e2, ok := loaded.entries[ch2.Hash]
	if !ok {
		t.Fatal("chunk2 hash not found in loaded index")
	}
	if e2.offset != 42 || e2.length != 43 || e2.flags != 0x01 {
		t.Errorf("chunk2 entry mismatch: offset=%d length=%d flags=%d", e2.offset, e2.length, e2.flags)
	}
}

func TestGetChunk_PackIntegrityCheck(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	ch := makeChunk([]byte("integrity test data for pack"))
	if err := fs.PutChunk(context.Background(), ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	if err := fs.createPack(context.Background(), []core.Hash{ch.Hash}); err != nil {
		t.Fatalf("createPack: %v", err)
	}

	if err := fs.DeleteChunk(context.Background(), ch.Hash); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}
	fs.chunkCache.Remove(ch.Hash)

	idx, err := fs.getPackIndex("pack-00000001")
	if err != nil {
		t.Fatalf("getPackIndex: %v", err)
	}
	entry, ok := idx.entries[ch.Hash]
	if !ok {
		t.Fatal("chunk not found in pack index")
	}

	packPath := fs.packPath("pack-00000001")
	f, err := os.OpenFile(packPath, os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open pack for corruption: %v", err)
	}
	if _, err := f.WriteAt([]byte("X"), entry.offset+1); err != nil {
		f.Close()
		t.Fatalf("corrupt pack: %v", err)
	}
	f.Close()

	_, err = fs.GetChunk(context.Background(), ch.Hash)
	if err == nil {
		t.Fatal("expected error for corrupted pack chunk, got nil")
	}
}

func TestListPackNames(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	names, err := fs.listPackNames()
	if err != nil {
		t.Fatalf("listPackNames on empty: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 pack names, got %d", len(names))
	}

	ch := makeChunk([]byte("list names test chunk"))
	if err := fs.PutChunk(context.Background(), ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}
	if err := fs.createPack(context.Background(), []core.Hash{ch.Hash}); err != nil {
		t.Fatalf("createPack: %v", err)
	}

	names, err = fs.listPackNames()
	if err != nil {
		t.Fatalf("listPackNames: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("expected 1 pack name, got %d: %v", len(names), names)
	}
	if names[0] != "pack-00000001" {
		t.Errorf("expected pack-00000001, got %q", names[0])
	}
}

func TestNextPackName(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	name, err := fs.nextPackName()
	if err != nil {
		t.Fatalf("nextPackName on empty: %v", err)
	}
	if name != "pack-00000001" {
		t.Errorf("expected pack-00000001, got %q", name)
	}

	dummyPath := filepath.Join(fs.packsDir(), "pack-00000005.pack")
	if err := os.WriteFile(dummyPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("create dummy pack: %v", err)
	}

	name, err = fs.nextPackName()
	if err != nil {
		t.Fatalf("nextPackName with existing: %v", err)
	}
	if name != "pack-00000006" {
		t.Errorf("expected pack-00000006, got %q", name)
	}
}

