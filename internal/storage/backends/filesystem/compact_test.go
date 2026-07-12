package filesystem

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/Alei-001/drift/internal/core"
)

func TestCompactChunks_PacksLoose(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	n := packThreshold + 10
	chunks := make([]*core.Chunk, n)
	reachable := make(map[core.Hash]bool, n)
	for i := 0; i < n; i++ {
		data := []byte(fmt.Sprintf("compact-pack-test-chunk-%d-%d", i, i))
		chunks[i] = makeChunk(data)
		if err := fs.PutChunk(context.Background(), chunks[i]); err != nil {
			t.Fatalf("PutChunk %d: %v", i, err)
		}
		reachable[chunks[i].Hash] = true
	}

	report, err := fs.CompactChunks(context.Background(), reachable, false)
	if err != nil {
		t.Fatalf("CompactChunks: %v", err)
	}
	if report.LoosePacked != n {
		t.Errorf("expected LoosePacked=%d, got %d", n, report.LoosePacked)
	}
	if report.PacksCreated != 1 {
		t.Errorf("expected PacksCreated=1, got %d", report.PacksCreated)
	}

	fs.chunkCache.Remove(chunks[0].Hash)
	for i, ch := range chunks {
		got, err := fs.GetChunk(context.Background(), ch.Hash)
		if err != nil {
			t.Fatalf("GetChunk %d after compact: %v", i, err)
		}
		if !bytes.Equal(got.Data, ch.Data) {
			t.Fatalf("chunk %d data mismatch after compact", i)
		}
	}
}

func TestCompactChunks_DeletesDeadLoose(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	alive := makeChunk([]byte("alive loose chunk"))
	dead := makeChunk([]byte("dead loose chunk"))

	if err := fs.PutChunk(context.Background(), alive); err != nil {
		t.Fatalf("PutChunk alive: %v", err)
	}
	if err := fs.PutChunk(context.Background(), dead); err != nil {
		t.Fatalf("PutChunk dead: %v", err)
	}

	reachable := map[core.Hash]bool{alive.Hash: true}

	report, err := fs.CompactChunks(context.Background(), reachable, false)
	if err != nil {
		t.Fatalf("CompactChunks: %v", err)
	}
	if report.LooseDeleted != 1 {
		t.Errorf("expected LooseDeleted=1, got %d", report.LooseDeleted)
	}

	ok, err := fs.HasChunk(context.Background(), alive.Hash)
	if err != nil {
		t.Fatalf("HasChunk alive: %v", err)
	}
	if !ok {
		t.Fatal("alive chunk should still exist")
	}

	ok, err = fs.HasChunk(context.Background(), dead.Hash)
	if err != nil {
		t.Fatalf("HasChunk dead: %v", err)
	}
	if ok {
		t.Fatal("dead chunk should have been deleted")
	}
}

func TestCompactChunks_RewritesPack(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	alive := makeChunk([]byte("alive chunk in pack"))
	dead := makeChunk([]byte("dead chunk in pack"))
	extraDead := makeChunk([]byte("extra dead chunk in pack"))

	allHashes := []core.Hash{alive.Hash, dead.Hash, extraDead.Hash}
	for _, h := range []*core.Chunk{alive, dead, extraDead} {
		if err := fs.PutChunk(context.Background(), h); err != nil {
			t.Fatalf("PutChunk: %v", err)
		}
	}

	if err := fs.createPack(context.Background(), allHashes); err != nil {
		t.Fatalf("createPack: %v", err)
	}
	for _, h := range allHashes {
		if err := fs.DeleteChunk(context.Background(), h); err != nil {
			t.Fatalf("DeleteChunk: %v", err)
		}
	}
	fs.chunkCache.Remove(alive.Hash)

	reachable := map[core.Hash]bool{alive.Hash: true}

	report, err := fs.CompactChunks(context.Background(), reachable, false)
	if err != nil {
		t.Fatalf("CompactChunks: %v", err)
	}
	if report.PacksRewritten != 1 {
		t.Errorf("expected PacksRewritten=1, got %d", report.PacksRewritten)
	}
	if report.PackDeadRemoved != 2 {
		t.Errorf("expected PackDeadRemoved=2, got %d", report.PackDeadRemoved)
	}

	got, err := fs.GetChunk(context.Background(), alive.Hash)
	if err != nil {
		t.Fatalf("GetChunk alive after rewrite: %v", err)
	}
	if !bytes.Equal(got.Data, alive.Data) {
		t.Fatal("alive chunk data mismatch after rewrite")
	}

	ok, err := fs.HasChunk(context.Background(), dead.Hash)
	if err != nil {
		t.Fatalf("HasChunk dead: %v", err)
	}
	if ok {
		t.Fatal("dead chunk should not be reachable after rewrite")
	}
}

func TestCompactChunks_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	alive := makeChunk([]byte("alive for dry-run"))
	dead := makeChunk([]byte("dead for dry-run"))

	if err := fs.PutChunk(context.Background(), alive); err != nil {
		t.Fatalf("PutChunk alive: %v", err)
	}
	if err := fs.PutChunk(context.Background(), dead); err != nil {
		t.Fatalf("PutChunk dead: %v", err)
	}

	reachable := map[core.Hash]bool{alive.Hash: true}

	report, err := fs.CompactChunks(context.Background(), reachable, true)
	if err != nil {
		t.Fatalf("CompactChunks dry-run: %v", err)
	}
	if report.LooseDeleted != 1 {
		t.Errorf("expected LooseDeleted=1 in dry-run, got %d", report.LooseDeleted)
	}

	ok, err := fs.HasChunk(context.Background(), dead.Hash)
	if err != nil {
		t.Fatalf("HasChunk dead: %v", err)
	}
	if !ok {
		t.Fatal("dead chunk should still exist after dry-run")
	}
}

func TestCompactChunks_BelowThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	ch := makeChunk([]byte("below threshold chunk"))
	if err := fs.PutChunk(context.Background(), ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}

	reachable := map[core.Hash]bool{ch.Hash: true}

	report, err := fs.CompactChunks(context.Background(), reachable, false)
	if err != nil {
		t.Fatalf("CompactChunks: %v", err)
	}
	if report.LoosePacked != 0 {
		t.Errorf("expected LoosePacked=0 below threshold, got %d", report.LoosePacked)
	}
	if report.LooseDeleted != 0 {
		t.Errorf("expected LooseDeleted=0, got %d", report.LooseDeleted)
	}
}

func TestCompactChunks_EmptyPack(t *testing.T) {
	tmpDir := t.TempDir()
	fs, err := NewFSStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFSStorage: %v", err)
	}
	defer fs.Close()

	ch := makeChunk([]byte("all dead in pack"))
	if err := fs.PutChunk(context.Background(), ch); err != nil {
		t.Fatalf("PutChunk: %v", err)
	}
	if err := fs.createPack(context.Background(), []core.Hash{ch.Hash}); err != nil {
		t.Fatalf("createPack: %v", err)
	}
	if err := fs.DeleteChunk(context.Background(), ch.Hash); err != nil {
		t.Fatalf("DeleteChunk: %v", err)
	}

	noReachable := make(map[core.Hash]bool)

	report, err := fs.CompactChunks(context.Background(), noReachable, false)
	if err != nil {
		t.Fatalf("CompactChunks: %v", err)
	}
	if report.PackDeadRemoved == 0 {
		t.Errorf("expected PackDeadRemoved > 0 for empty pack, got %d", report.PackDeadRemoved)
	}

	names, err := fs.listPackNames()
	if err != nil {
		t.Fatalf("listPackNames: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected no packs left, got %v", names)
	}
}
