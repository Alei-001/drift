package app

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/drift/drift/internal/config"
	"github.com/drift/drift/internal/core"
	"github.com/drift/drift/internal/storage"
	"github.com/drift/drift/internal/worktree"
)

// newTestApp builds a fully initialized App rooted in a temp dir with a
// default config (so Save can run). The caller writes files under dir.
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

// addPath invokes Add with an absolute path (ExpandAddPaths resolves relative
// to the process cwd, which is not the repo root under test).
func addPath(t *testing.T, a *App, rel string) {
	t.Helper()
	if _, err := a.Add([]string{filepath.Join(a.dir, filepath.FromSlash(rel))}); err != nil {
		t.Fatalf("Add %s failed: %v", rel, err)
	}
}

// stagedPaths returns the set of paths with a non-Unmodified staging status,
// sorted. Used to assert status output after add/unstage.
func stagedPaths(s *core.Status) []string {
	var out []string
	for p, fs := range *s {
		if fs.Staging != core.Unmodified {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func untrackedPaths(s *core.Status) []string {
	var out []string
	for p, fs := range *s {
		if fs.Worktree == core.Untracked {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// TestUnstageAll_PreservesFullSnapshot is the regression test for the bug
// where `drift unstage` emptied the index, then `drift add .` produced a
// partial index, causing committed-but-unstaged files to falsely show as
// both staged-deleted and untracked.
func TestUnstageAll_PreservesFullSnapshot(t *testing.T) {
	a := newTestApp(t)
	// Seed an initial commit with several files.
	writeFile(t, a.dir, "a.txt", "a")
	writeFile(t, a.dir, "c.txt", "c")
	writeFile(t, a.dir, "cesho.md", "cesho")
	writeFile(t, a.dir, "ml/gv.doc", "gv")
	writeFile(t, a.dir, "ml/mc.ai", "mc")
	writeFile(t, a.dir, "p.txt", "p")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	// Make real changes: delete a.txt + c.txt, modify p.txt, add new files.
	deleteFile(t, a.dir, "a.txt")
	deleteFile(t, a.dir, "c.txt")
	writeFile(t, a.dir, "p.txt", "p-modified")
	writeFile(t, a.dir, "zc/ji.ts", "ji")
	writeFile(t, a.dir, "zc/lj.txt", "lj")

	// Stage everything (mirrors `drift add .`).
	if _, err := a.Add([]string{"."}); err != nil {
		t.Fatalf("Add . failed: %v", err)
	}
	// Unstage all (mirrors `drift unstage`).
	if err := a.ClearStaging(); err != nil {
		t.Fatalf("ClearStaging failed: %v", err)
	}
	// Re-add everything (mirrors `drift add .`).
	if _, err := a.Add([]string{"."}); err != nil {
		t.Fatalf("Add . after unstage failed: %v", err)
	}

	status, err := a.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	wantStaged := map[string]core.StatusCode{
		"a.txt":     core.Deleted,
		"c.txt":     core.Deleted,
		"p.txt":     core.Modified,
		"zc/ji.ts":  core.Added,
		"zc/lj.txt": core.Added,
	}
	for p, want := range wantStaged {
		fs, ok := (*status)[p]
		if !ok {
			t.Errorf("status missing %s", p)
			continue
		}
		if fs.Staging != want {
			t.Errorf("staging %s = %q, want %q", p, fs.Staging, want)
		}
	}
	// No committed file should reappear as staged-deleted or untracked.
	for _, p := range []string{"cesho.md", "ml/gv.doc", "ml/mc.ai"} {
		if fs, ok := (*status)[p]; ok && (fs.Staging == core.Deleted || fs.Worktree == core.Untracked) {
			t.Errorf("unchanged committed file %s wrongly reported: staging=%q worktree=%q", p, fs.Staging, fs.Worktree)
		}
	}
	if got := untrackedPaths(status); len(got) != 0 {
		t.Errorf("expected no untracked files, got %v", got)
	}
}

// TestUnstageAll_ThenSave_NoDataLoss verifies that the unstage->add->save
// sequence does not silently drop unchanged committed files from history.
func TestUnstageAll_ThenSave_NoDataLoss(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "keep1.txt", "k1")
	writeFile(t, a.dir, "keep2.txt", "k2")
	writeFile(t, a.dir, "del.txt", "d")
	writeFile(t, a.dir, "mod.txt", "m")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	deleteFile(t, a.dir, "del.txt")
	writeFile(t, a.dir, "mod.txt", "m-modified")

	if _, err := a.Add([]string{"."}); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := a.ClearStaging(); err != nil {
		t.Fatalf("ClearStaging failed: %v", err)
	}
	if _, err := a.Add([]string{"."}); err != nil {
		t.Fatalf("Add after unstage failed: %v", err)
	}
	res, err := a.Save("second", SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// The new commit tree must still contain the unchanged files.
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
	for _, want := range []string{"keep1.txt", "keep2.txt", "mod.txt"} {
		if !have[want] {
			t.Errorf("commit %s lost unchanged file %s (data loss)", res.ID, want)
		}
	}
	if have["del.txt"] {
		t.Errorf("commit %s should have dropped del.txt", res.ID)
	}
}

// TestUnstagePath_RestoresCommittedState verifies `drift unstage <path>` for
// a previously-committed file resets it to its committed hash (not removes it).
func TestUnstagePath_RestoresCommittedState(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	writeFile(t, a.dir, "b.txt", "b")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	// Modify a.txt and stage it.
	writeFile(t, a.dir, "a.txt", "a-modified")
	addPath(t, a, "a.txt")
	// Unstage just a.txt.
	unstaged, notFound, err := a.Unstage([]string{"a.txt"})
	if err != nil {
		t.Fatalf("Unstage failed: %v", err)
	}
	if len(notFound) != 0 {
		t.Errorf("unexpected notFound: %v", notFound)
	}
	if len(unstaged) != 1 || unstaged[0] != "a.txt" {
		t.Errorf("unstaged = %v, want [a.txt]", unstaged)
	}

	// After unstage, a.txt must be back to committed hash (worktree is still
	// modified, so it should show as Worktree=Modified, Staging=Unmodified).
	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	e, err := idx.Entry("a.txt")
	if err != nil {
		t.Fatalf("a.txt missing from index after unstage: %v", err)
	}
	commit, _ := a.currentCommit()
	tree, _ := a.store.GetTree(commit.TreeHash)
	blobs, _ := core.NewTreeReader(a.store).ListBlobs(tree, "")
	wantHash := ""
	for _, b := range blobs {
		if b.Path == "a.txt" {
			wantHash = b.Hash
		}
	}
	if e.Hash != wantHash {
		t.Errorf("a.txt index hash = %s, want committed %s", e.Hash, wantHash)
	}

	status, err := a.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	fs := (*status)["a.txt"]
	if fs.Staging != core.Unmodified {
		t.Errorf("a.txt staging = %q, want Unmodified", fs.Staging)
	}
	if fs.Worktree != core.Modified {
		t.Errorf("a.txt worktree = %q, want Modified", fs.Worktree)
	}
}

// TestUnstagePath_NewlyStagedAdd_BecomesUntracked verifies unstaging a file
// that was staged as a brand-new add removes it from the index (untracked again).
func TestUnstagePath_NewlyStagedAdd_BecomesUntracked(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	writeFile(t, a.dir, "new.txt", "new")
	addPath(t, a, "new.txt")
	if _, _, err := a.Unstage([]string{"new.txt"}); err != nil {
		t.Fatalf("Unstage failed: %v", err)
	}

	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if idx.Has("new.txt") {
		t.Errorf("new.txt should be removed from index after unstaging a new add")
	}
	status, err := a.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if fs := (*status)["new.txt"]; fs == nil || fs.Worktree != core.Untracked {
		t.Errorf("new.txt should be untracked after unstage, got %+v", fs)
	}
}

// TestUnstageAll_FreshRepo_NoCommit ensures ClearStaging on a repo with no
// commits yields an empty index (no panic, no error).
func TestUnstageAll_FreshRepo_NoCommit(t *testing.T) {
	a := newTestApp(t)
	writeFile(t, a.dir, "a.txt", "a")
	addPath(t, a, "a.txt")
	if err := a.ClearStaging(); err != nil {
		t.Fatalf("ClearStaging failed: %v", err)
	}
	var idx core.Index
	if err := a.store.LoadIndex(&idx); err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}
	if len(idx.Entries) != 0 {
		t.Errorf("expected empty index in fresh repo, got %d entries", len(idx.Entries))
	}
}

// TestBuildIndexFromCommit_KeepsUnchangedFiles is a direct unit test for the
// helper that backs ClearStaging, asserting it produces a full snapshot.
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
	// Sanity: the worktree helper is reusable standalone.
	if _, err := worktree.New(a.store, a.dir, "").BuildIndexFromCommit(); err != nil {
		t.Errorf("standalone BuildIndexFromCommit failed: %v", err)
	}
}

// TestSave_TagConflict_BeforePersistence verifies that a tag conflict produces
// a TagWarning but does not prevent the save. A retry with a different tag
// should succeed.
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

	// Retry with a new tag should succeed without warning.
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

// TestSave_TagConflict_NonFatal verifies that a duplicate tag produces a
// TagWarning on the SaveResult but does not fail the save.
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

// TestSave_PreservesUnchangedSubdirFiles verifies that after add + save,
// unchanged files in subdirectories remain in the commit tree and the
// status shows clean (no spurious staged additions).
func TestSave_PreservesUnchangedSubdirFiles(t *testing.T) {
	a := newTestApp(t)

	// Seed initial commit with root-level and subdirectory files.
	writeFile(t, a.dir, "cesho.md", "cesho")
	writeFile(t, a.dir, "g.txt", "g")
	writeFile(t, a.dir, "v.md", "v")
	writeFile(t, a.dir, "2/w.txt", "w")
	writeFile(t, a.dir, "zc/ji.ts", "ji")
	writeFile(t, a.dir, "zc/lj.txt", "lj")
	if _, err := a.Save("init", SaveOptions{}); err != nil {
		t.Fatalf("initial Save failed: %v", err)
	}

	// Verify status is clean.
	s, err := a.Status()
	if err != nil {
		t.Fatalf("Status after init failed: %v", err)
	}
	if !s.IsClean() {
		t.Errorf("expected clean status after init save, got %d entries", len(*s))
	}

	// Make changes: modify cesho.md, delete g.txt, modify v.md, add ces.txt.
	writeFile(t, a.dir, "cesho.md", "cesho-modified")
	deleteFile(t, a.dir, "g.txt")
	writeFile(t, a.dir, "v.md", "v-modified")
	writeFile(t, a.dir, "ces.txt", "ces")

	// Stage all changes (drift add .)
	if _, err := a.Add([]string{"."}); err != nil {
		t.Fatalf("Add . failed: %v", err)
	}

	// Save
	res, err := a.Save("test color", SaveOptions{})
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the commit tree contains all expected files.
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

	// Deleted g.txt must not appear in the new commit.
	if have["g.txt"] {
		t.Errorf("commit %s still contains deleted file g.txt", res.ID)
	}
}

// TestSave_TagNewTag verifies that a new tag is successfully created on save.
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
