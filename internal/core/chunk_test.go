package core

import "testing"

func TestChunkFlag_Values(t *testing.T) {
	if ChunkFlagNone != 0 {
		t.Errorf("ChunkFlagNone = %d, want 0", ChunkFlagNone)
	}
	if ChunkFlagCompressed != 1 {
		t.Errorf("ChunkFlagCompressed = %d, want 1", ChunkFlagCompressed)
	}
}

func TestChunk_Fields(t *testing.T) {
	c := Chunk{
		Hash:  Hash{0x01, 0x02},
		Size:  100,
		Data:  []byte("hello"),
		Flags: ChunkFlagNone,
	}
	if c.Hash != (Hash{0x01, 0x02}) {
		t.Errorf("Hash: got %v, want %v", c.Hash, Hash{0x01, 0x02})
	}
	if c.Size != 100 {
		t.Errorf("Size: got %d, want 100", c.Size)
	}
	if string(c.Data) != "hello" {
		t.Errorf("Data: got %q, want %q", c.Data, "hello")
	}
	if c.Flags != ChunkFlagNone {
		t.Errorf("Flags: got %d, want %d", c.Flags, ChunkFlagNone)
	}
}

func TestChunk_CompressedFlag(t *testing.T) {
	c := Chunk{
		Data:  []byte("compressed"),
		Flags: ChunkFlagCompressed,
	}
	if c.Flags != ChunkFlagCompressed {
		t.Errorf("Flags: got %d, want %d", c.Flags, ChunkFlagCompressed)
	}
	if c.Flags&ChunkFlagCompressed == 0 {
		t.Error("expected ChunkFlagCompressed bit to be set")
	}
}

func TestChunk_Empty(t *testing.T) {
	c := Chunk{}
	if c.Hash != (Hash{}) {
		t.Error("expected zero Hash for empty chunk")
	}
	if c.Size != 0 {
		t.Errorf("expected Size 0, got %d", c.Size)
	}
	if c.Data != nil {
		t.Error("expected nil Data for empty chunk")
	}
	if c.Flags != ChunkFlagNone {
		t.Errorf("expected ChunkFlagNone, got %d", c.Flags)
	}
}
