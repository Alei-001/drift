package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/drift/drift/internal/worktree"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	store := storage.NewStore(dir)
	cfg := config.DefaultConfig()
	a := New(store, cfg, dir)
	if err := a.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return a
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func deleteFile(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

func TestBuildIndexFromCommit_KeepsUnchangedFiles(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	writeFile(t, a.dir, "ml/b.txt", "b")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}
	idx, err := a.wt.BuildIndexFromCommit()
	if err != nil {
		t.Fatalf("BuildIndexFromCommit failed: %v", err)
	}
	if !idx.Has("a.txt") || !idx.Has("ml/b.txt") {
		t.Errorf("BuildIndexFromCommit missing committed files, entries: %v", idx.Entries)
	}
	if _, err := worktree.New(a.store, a.dir, "").BuildIndexFromCommit(); err != nil {
		t.Errorf("standalone BuildIndexFromCommit failed: %v", err)
	}
}

func TestSave_TagConflict_BeforePersistence(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	res, err := a.Save("init", SaveOptions{Tag: "v1"})
	if err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	writeFile(t, a.dir, "a.txt", "a-modified")

	res2, err := a.Save("second", SaveOptions{Tag: "v1"})
	if err != nil {
		t.Fatalf("second Save (duplicate tag) failed: %v", err)
	}
	if res2.TagWarning == nil {
		t.Fatalf("expected TagWarning for duplicate tag, got nil")
	}

	writeFile(t, a.dir, "a.txt", "a-modified3")
	res3, err := a.Save("third", SaveOptions{Tag: "v2"})
	if err != nil {
		t.Fatalf("retry Save with different tag failed: %v", err)
	}
	if res3.TagWarning != nil {
		t.Errorf("unexpected TagWarning: %v", res3.TagWarning)
	}
	if res3.ID == res.ID {
		t.Errorf("retry saved new commit ID %s, expected different from %s", res3.ID, res.ID)
	}
}

func TestSave_TagConflict_NonFatal(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	res, err := a.Save("init", SaveOptions{Tag: "v1"})
	if err != nil {
		t.Fatalf("first Save failed: %v", err)
	}
	if res.TagWarning != nil {
		t.Fatalf("unexpected TagWarning on first save: %v", res.TagWarning)
	}

	writeFile(t, a.dir, "a.txt", "a-modified")

	res2, err := a.Save("second", SaveOptions{Tag: "v1"})
	if err != nil {
		t.Fatalf("second Save failed: %v", err)
	}
	if res2.TagWarning == nil {
		t.Errorf("expected TagWarning for duplicate tag, got nil")
	}
	if res2.ID == res.ID {
		t.Errorf("second save produced same commit ID %s", res2.ID)
	}

	writeFile(t, a.dir, "a.txt", "a-modified2")
	res3, err := a.Save("third", SaveOptions{Tag: "v2"})
	if err != nil {
		t.Fatalf("third Save failed: %v", err)
	}
	if res3.TagWarning != nil {
		t.Errorf("unexpected TagWarning: %v", res3.TagWarning)
	}
}

func TestSave_PreservesUnchangedSubdirFiles(t *testing.T) {
	a := newTestApp(t)

	writeFile(t, a.dir, "cesho.md", "cesho")
	writeFile(t, a.dir, "g.txt", "g")
	writeFile(t, a.dir, "v.md", "v")
	writeFile(t, a.dir, "2/w.txt", "w")
	writeFile(t, a.dir, "zc/ji.ts", "ji")
	writeFile(t, a.dir, "zc/lj.txt", "lj")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	s, err := a.Status()
	if err != nil {
		t.Fatalf("Status after init failed: %v", err)
	}
	if !s.IsClean() {
		t.Errorf("expected clean status after init save, got %d entries", len(*s))
	}

	writeFile(t, a.dir, "cesho.md", "cesho-modified")
	deleteFile(t, a.dir, "g.txt")
	writeFile(t, a.dir, "v.md", "v-modified")
	writeFile(t, a.dir, "ces.txt", "ces")

	res, err := a.Save("test color", SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	commit, err := a.ResolveCommit(res.ID)
	if err != nil {
		t.Fatalf("ResolveCommit failed: %v", err)
	}
	tree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		t.Fatalf("GetTree failed: %v", err)
	}
	blobs, err := core.NewTreeReader(a.store).ListBlobs(tree, "")
	if err != nil {
		t.Fatalf("ListBlobs failed: %v", err)
	}
	have := make(map[string]bool, len(blobs))
	for _, b := range blobs {
		have[b.Path] = true
	}
	for _, want := range []string{"2/w.txt", "zc/ji.ts", "zc/lj.txt", "cesho.md", "v.md", "ces.txt"} {
		if !have[want] {
			t.Errorf("commit %s lost file %s", res.ID, want)
		}
	}
	if have["g.txt"] {
		t.Errorf("commit %s still contains deleted file g.txt", res.ID)
	}
}

func TestSave_TagNewTag(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	writeFile(t, a.dir, "a.txt", "a2")
	res, err := a.Save("updated", SaveOptions{Tag: "v1"})
	if err != nil {
		t.Fatalf("Save with new tag failed: %v", err)
	}
	hash, err := a.store.GetRef("tags/v1")
	if err != nil || hash == "" {
		t.Fatalf("tag v1 should exist after Save with new tag, res=%+v", res)
	}
}

func TestSave_AutoDetectsAllChanges(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "keep.txt", "keep")
	writeFile(t, a.dir, "mod.txt", "old")
	writeFile(t, a.dir, "del.txt", "del")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	writeFile(t, a.dir, "mod.txt", "new")
	deleteFile(t, a.dir, "del.txt")
	writeFile(t, a.dir, "added.txt", "added")

	res, err := a.Save("auto-detected", SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if len(res.ChangedPaths) != 3 {
		t.Errorf("expected 3 changed paths, got %d: %v", len(res.ChangedPaths), res.ChangedPaths)
	}

	commit, err := a.ResolveCommit(res.ID)
	if err != nil {
		t.Fatalf("ResolveCommit failed: %v", err)
	}
	tree, err := a.store.GetTree(commit.TreeHash)
	if err != nil {
		t.Fatalf("GetTree failed: %v", err)
	}
	blobs, _ := core.NewTreeReader(a.store).ListBlobs(tree, "")
	have := make(map[string]bool)
	for _, b := range blobs {
		have[b.Path] = true
	}
	if !have["keep.txt"] {
		t.Error("commit lost unchanged file keep.txt")
	}
	if !have["added.txt"] {
		t.Error("commit did not include newly added file added.txt")
	}
	if have["del.txt"] {
		t.Error("commit still contains deleted file del.txt")
	}
}

func TestSave_NothingChanged(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}
	_, err := a.Save("nothing new", SaveOptions{})
	if err == nil {
		t.Fatal("expected error for unchanged save, got nil")
	}
}
