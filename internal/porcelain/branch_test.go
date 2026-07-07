package porcelain

import (
	"context"
	"strings"
	"testing"

	"github.com/Alei-001/drift/internal/core"
	"github.com/Alei-001/drift/internal/storage/backends/memory"
)

func setupBranchStore() *memory.MemoryStorage {
	store := memory.NewMemoryStorage()
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{},
	})
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/main",
	})
	return store
}

func TestCreateBranch_FromHead(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	targetHash := core.Hash{0x12, 0xab}
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: targetHash,
	})

	err := CreateBranch(context.Background(), store, dir, "feature")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	ref, err := store.GetRef(context.Background(), "heads/feature")
	if err != nil {
		t.Fatalf("GetRef heads/feature failed: %v", err)
	}
	if ref.Target != targetHash {
		t.Errorf("expected target %x, got %x", targetHash, ref.Target)
	}
}

func TestCreateBranch_AlreadyExists(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	err := CreateBranch(context.Background(), store, dir, "feature")
	if err != nil {
		t.Fatalf("first CreateBranch failed: %v", err)
	}

	err = CreateBranch(context.Background(), store, dir, "feature")
	if err == nil {
		t.Fatal("expected error for duplicate branch, got nil")
	}
}

func TestCreateBranch_InvalidName(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	if err := CreateBranch(context.Background(), store, dir, ""); err == nil {
		t.Error("expected error for empty name, got nil")
	}

	if err := CreateBranch(context.Background(), store, dir, "foo..bar"); err == nil {
		t.Error("expected error for name with '..', got nil")
	}

	// Hierarchical branch names (e.g. "feature/foo") follow Git semantics
	// and are allowed by refname.Validate.
	if err := CreateBranch(context.Background(), store, dir, "feature/foo"); err != nil {
		t.Errorf("expected hierarchical name to be allowed, got: %v", err)
	}
}

func TestListBranches(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	if err := CreateBranch(context.Background(), store, dir, "feature"); err != nil {
		t.Fatalf("CreateBranch feature failed: %v", err)
	}
	if err := CreateBranch(context.Background(), store, dir, "dev"); err != nil {
		t.Fatalf("CreateBranch dev failed: %v", err)
	}

	branches, current, err := ListBranches(context.Background(), store)
	if err != nil {
		t.Fatalf("ListBranches failed: %v", err)
	}

	if len(branches) != 3 {
		t.Errorf("expected 3 branches, got %d", len(branches))
	}

	if current != "main" {
		t.Errorf("expected current branch 'main', got '%s'", current)
	}
}

func TestDeleteBranch_Success(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	if err := CreateBranch(context.Background(), store, dir, "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	err := DeleteBranch(context.Background(), store, dir, "feature")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	if _, err := store.GetRef(context.Background(), "heads/feature"); err == nil {
		t.Error("expected heads/feature to be deleted, but it still exists")
	}
}

func TestDeleteBranch_NotFound(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	err := DeleteBranch(context.Background(), store, dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got '%s'", err.Error())
	}
}

func TestDeleteBranch_CurrentBranch(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "feature")
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/feature",
	})

	err := DeleteBranch(context.Background(), store, dir, "feature")
	if err == nil {
		t.Fatal("expected error for deleting current branch, got nil")
	}
	if !strings.Contains(err.Error(), "cannot delete the current branch") {
		t.Errorf("expected error containing 'cannot delete the current branch', got '%s'", err.Error())
	}
}

func TestDeleteBranch_MainProtected(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "other")
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/other",
	})

	err := DeleteBranch(context.Background(), store, dir, "main")
	if err == nil {
		t.Fatal("expected error for deleting main, got nil")
	}
	if !strings.Contains(err.Error(), "cannot delete 'main'") {
		t.Errorf("expected error containing \"cannot delete 'main'\", got '%s'", err.Error())
	}
}

func TestDeleteBranch_EmptyName(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	err := DeleteBranch(context.Background(), store, dir, "")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestRenameBranch_NonCurrent(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	if err := CreateBranch(context.Background(), store, dir, "feature"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	err := RenameBranch(context.Background(), store, dir, "feature", "dev")
	if err != nil {
		t.Fatalf("RenameBranch failed: %v", err)
	}

	if _, err := store.GetRef(context.Background(), "heads/feature"); err == nil {
		t.Error("expected heads/feature to be removed, but it still exists")
	}
	newRef, err := store.GetRef(context.Background(), "heads/dev")
	if err != nil {
		t.Fatalf("GetRef heads/dev failed: %v", err)
	}
	if newRef.Name != "heads/dev" {
		t.Errorf("expected ref name 'heads/dev', got '%s'", newRef.Name)
	}

	// HEAD should be unchanged (still points to main).
	headRef, _ := store.GetRef(context.Background(), "HEAD")
	if headRef.SymRef != "heads/main" {
		t.Errorf("expected HEAD still at 'heads/main', got '%s'", headRef.SymRef)
	}
}

func TestRenameBranch_CurrentBranch_UpdatesHEAD(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "feature")
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/feature",
	})

	err := RenameBranch(context.Background(), store, dir, "feature", "dev")
	if err != nil {
		t.Fatalf("RenameBranch failed: %v", err)
	}

	if _, err := store.GetRef(context.Background(), "heads/feature"); err == nil {
		t.Error("expected heads/feature to be removed, but it still exists")
	}
	if _, err := store.GetRef(context.Background(), "heads/dev"); err != nil {
		t.Errorf("expected heads/dev to exist, got error: %v", err)
	}
	headRef, _ := store.GetRef(context.Background(), "HEAD")
	if headRef.SymRef != "heads/dev" {
		t.Errorf("expected HEAD SymRef 'heads/dev', got '%s'", headRef.SymRef)
	}
}

func TestRenameBranch_TargetExists(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "feature")
	CreateBranch(context.Background(), store, dir, "dev")

	err := RenameBranch(context.Background(), store, dir, "feature", "dev")
	if err == nil {
		t.Fatal("expected error for existing target name, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error containing 'already exists', got '%s'", err.Error())
	}

	// Both original branches should still be intact.
	if _, err := store.GetRef(context.Background(), "heads/feature"); err != nil {
		t.Error("heads/feature should still exist after failed rename")
	}
}

func TestRenameBranch_NotFound(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	err := RenameBranch(context.Background(), store, dir, "nonexistent", "dev")
	if err == nil {
		t.Fatal("expected error for non-existent source branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got '%s'", err.Error())
	}
}

func TestRenameBranch_MainProtected(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "other")
	store.SetRef(context.Background(), "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		SymRef: "heads/other",
	})

	err := RenameBranch(context.Background(), store, dir, "main", "trunk")
	if err == nil {
		t.Fatal("expected error for renaming main, got nil")
	}
	if !strings.Contains(err.Error(), "cannot rename 'main'") {
		t.Errorf("expected error containing \"cannot rename 'main'\", got '%s'", err.Error())
	}
}

func TestRenameBranch_SameName_NoOp(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "feature")

	err := RenameBranch(context.Background(), store, dir, "feature", "feature")
	if err != nil {
		t.Fatalf("rename to same name should be a no-op, got: %v", err)
	}
	if _, err := store.GetRef(context.Background(), "heads/feature"); err != nil {
		t.Error("heads/feature should still exist after same-name rename")
	}
}

func TestRenameBranch_SameName_NonExistent(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()

	// Renaming a non-existent branch to itself must NOT silently succeed;
	// the source branch existence check runs before the same-name no-op.
	err := RenameBranch(context.Background(), store, dir, "ghost", "ghost")
	if err == nil {
		t.Fatal("expected error for non-existent branch, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error containing 'not found', got '%s'", err.Error())
	}
}

func TestRenameBranch_InvalidNewName(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "feature")

	if err := RenameBranch(context.Background(), store, dir, "feature", "foo..bar"); err == nil {
		t.Error("expected error for new name with '..', got nil")
	}
	// Hierarchical names are allowed (Git semantics).
	if err := RenameBranch(context.Background(), store, dir, "feature", "release/v1"); err != nil {
		t.Errorf("expected hierarchical new name to be allowed, got: %v", err)
	}
}

func TestRenameBranch_PreservesTarget(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	targetHash := core.Hash{0x12, 0xab}
	store.SetRef(context.Background(), "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: targetHash,
	})
	CreateBranch(context.Background(), store, dir, "feature")
	// Point feature at a distinct target.
	store.SetRef(context.Background(), "heads/feature", &core.Reference{
		Name:   "heads/feature",
		Type:   core.RefTypeBranch,
		Target: targetHash,
	})

	if err := RenameBranch(context.Background(), store, dir, "feature", "dev"); err != nil {
		t.Fatalf("RenameBranch failed: %v", err)
	}
	devRef, _ := store.GetRef(context.Background(), "heads/dev")
	if devRef.Target != targetHash {
		t.Errorf("expected target %x preserved, got %x", targetHash, devRef.Target)
	}
}

func TestRenameBranch_EmptyNames(t *testing.T) {
	store := setupBranchStore()
	dir := t.TempDir()
	CreateBranch(context.Background(), store, dir, "feature")

	if err := RenameBranch(context.Background(), store, dir, "", "dev"); err == nil {
		t.Error("expected error for empty old name, got nil")
	}
	if err := RenameBranch(context.Background(), store, dir, "feature", ""); err == nil {
		t.Error("expected error for empty new name, got nil")
	}
}

func TestResolveBranchTips_Linear(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	// Create snapshots: s1 (root) -> s2 -> s3
	s1 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{1}},
		Message:   "first",
		Timestamp: 100,
	}
	s2 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{2}},
		PrevID:    &core.SnapshotID{Hash: core.Hash{1}},
		Message:   "second",
		Timestamp: 200,
	}
	s3 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{3}},
		PrevID:    &core.SnapshotID{Hash: core.Hash{2}},
		Message:   "third",
		Timestamp: 300,
	}
	store.PutSnapshot(ctx, s1)
	store.PutSnapshot(ctx, s2)
	store.PutSnapshot(ctx, s3)

	// main branch points at s3
	store.SetRef(ctx, "heads/main", &core.Reference{
		Name:   "heads/main",
		Type:   core.RefTypeBranch,
		Target: core.Hash{3},
	})

	result, err := ResolveBranchTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveBranchTips: %v", err)
	}

	// Only s3 (the tip) gets labeled; s1 and s2 are inherited, so they get
	// no entry. This is the git --decorate=short semantic: the branch column
	// shows where the branch head sits, leaving the rest of the chain blank.
	if names, ok := result[core.Hash{1}.String()]; ok {
		t.Errorf("s1: expected no entry (inherited), got %v", names)
	}
	if names, ok := result[core.Hash{2}.String()]; ok {
		t.Errorf("s2: expected no entry (inherited), got %v", names)
	}
	if names := result[core.Hash{3}.String()]; len(names) != 1 || names[0] != "main" {
		t.Errorf("s3: got %v, want [main]", names)
	}
}

func TestResolveBranchTips_MultipleTipsAtSameSnapshot(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	// s1 (root) -> s2
	s1 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{1}},
		Message:   "root",
		Timestamp: 100,
	}
	s2 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{2}},
		PrevID:    &core.SnapshotID{Hash: core.Hash{1}},
		Message:   "shared tip",
		Timestamp: 200,
	}
	store.PutSnapshot(ctx, s1)
	store.PutSnapshot(ctx, s2)

	// Two branches both point at s2 — this is the "many branches share a
	// commit" case the branch column must show without overflow.
	store.SetRef(ctx, "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: core.Hash{2}})
	store.SetRef(ctx, "heads/dev", &core.Reference{Name: "heads/dev", Type: core.RefTypeBranch, Target: core.Hash{2}})

	result, err := ResolveBranchTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveBranchTips: %v", err)
	}

	// s2 should list BOTH branches, sorted alphabetically.
	if names := result[core.Hash{2}.String()]; len(names) != 2 || names[0] != "dev" || names[1] != "main" {
		t.Errorf("s2: got %v, want [dev main] (sorted)", names)
	}
	// s1 is inherited by both → no entry.
	if names, ok := result[core.Hash{1}.String()]; ok {
		t.Errorf("s1: expected no entry (inherited), got %v", names)
	}
}

func TestResolveBranchTips_DivergedBranches(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	// s1 (root) -> s2 -> s3 (main tip)
	//                \-> s4 (feature tip)
	s1 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{1}},
		Message:   "root",
		Timestamp: 100,
	}
	s2 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{2}},
		PrevID:    &core.SnapshotID{Hash: core.Hash{1}},
		Message:   "shared",
		Timestamp: 200,
	}
	s3 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{3}},
		PrevID:    &core.SnapshotID{Hash: core.Hash{2}},
		Message:   "main tip",
		Timestamp: 300,
	}
	s4 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{4}},
		PrevID:    &core.SnapshotID{Hash: core.Hash{2}},
		Message:   "feature tip",
		Timestamp: 250,
	}
	store.PutSnapshot(ctx, s1)
	store.PutSnapshot(ctx, s2)
	store.PutSnapshot(ctx, s3)
	store.PutSnapshot(ctx, s4)

	store.SetRef(ctx, "heads/main", &core.Reference{Name: "heads/main", Type: core.RefTypeBranch, Target: core.Hash{3}})
	store.SetRef(ctx, "heads/feature", &core.Reference{Name: "heads/feature", Type: core.RefTypeBranch, Target: core.Hash{4}})

	result, err := ResolveBranchTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveBranchTips: %v", err)
	}

	// Only tips get labeled. s1 and s2 are shared ancestors — they get NO
	// entry, so the log user can see at a glance that branches diverged at s2.
	if names, ok := result[core.Hash{1}.String()]; ok {
		t.Errorf("s1: expected no entry (shared ancestor), got %v", names)
	}
	if names, ok := result[core.Hash{2}.String()]; ok {
		t.Errorf("s2: expected no entry (divergence point), got %v", names)
	}
	if names := result[core.Hash{3}.String()]; len(names) != 1 || names[0] != "main" {
		t.Errorf("s3: got %v, want [main]", names)
	}
	if names := result[core.Hash{4}.String()]; len(names) != 1 || names[0] != "feature" {
		t.Errorf("s4: got %v, want [feature]", names)
	}
}

func TestResolveBranchTips_NoBranches(t *testing.T) {
	ctx := context.Background()
	store := memory.NewMemoryStorage()
	store.SetRef(ctx, "HEAD", &core.Reference{
		Name: "HEAD",
		Type: core.RefTypeHead,
	})
	result, err := ResolveBranchTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveBranchTips: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestWalkSnapshotChain_FullChain(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	// s1 (root) -> s2 -> s3, main tip at s3.
	h1, h2, h3 := core.Hash{1}, core.Hash{2}, core.Hash{3}
	s1 := &core.Snapshot{ID: core.SnapshotID{Hash: h1}, Message: "first", Timestamp: 100}
	s2 := &core.Snapshot{ID: core.SnapshotID{Hash: h2}, PrevID: &core.SnapshotID{Hash: h1}, Message: "second", Timestamp: 200}
	s3 := &core.Snapshot{ID: core.SnapshotID{Hash: h3}, PrevID: &core.SnapshotID{Hash: h2}, Message: "third", Timestamp: 300}
	store.PutSnapshot(ctx, s1)
	store.PutSnapshot(ctx, s2)
	store.PutSnapshot(ctx, s3)

	// Walk from s3 — should return [s3, s2, s1] in chain order (newest first).
	summaries, err := WalkSnapshotChain(ctx, store, h3)
	if err != nil {
		t.Fatalf("WalkSnapshotChain: %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(summaries))
	}
	if summaries[0].ID.Hash != h3 {
		t.Errorf("first: got %x, want 03", summaries[0].ID.Hash)
	}
	if summaries[1].ID.Hash != h2 {
		t.Errorf("second: got %x, want 02", summaries[1].ID.Hash)
	}
	if summaries[2].ID.Hash != h1 {
		t.Errorf("third: got %x, want 01", summaries[2].ID.Hash)
	}
}

func TestWalkSnapshotChain_IncludesInheritedCommits(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	// s1 (root) -> s2 -> s3 (main tip)
	//                \-> s4 (feature tip)
	// Walking from feature tip s4 should return [s4, s2, s1] — the shared
	// ancestors s2 and s1 are included (git log semantics).
	h1, h2, h3, h4 := core.Hash{1}, core.Hash{2}, core.Hash{3}, core.Hash{4}
	s1 := &core.Snapshot{ID: core.SnapshotID{Hash: h1}, Message: "root", Timestamp: 100}
	s2 := &core.Snapshot{ID: core.SnapshotID{Hash: h2}, PrevID: &core.SnapshotID{Hash: h1}, Message: "shared", Timestamp: 200}
	s3 := &core.Snapshot{ID: core.SnapshotID{Hash: h3}, PrevID: &core.SnapshotID{Hash: h2}, Message: "main tip", Timestamp: 300}
	s4 := &core.Snapshot{ID: core.SnapshotID{Hash: h4}, PrevID: &core.SnapshotID{Hash: h2}, Message: "feature tip", Timestamp: 250}
	store.PutSnapshot(ctx, s1)
	store.PutSnapshot(ctx, s2)
	store.PutSnapshot(ctx, s3)
	store.PutSnapshot(ctx, s4)

	summaries, err := WalkSnapshotChain(ctx, store, h4)
	if err != nil {
		t.Fatalf("WalkSnapshotChain: %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries (s4, s2, s1), got %d", len(summaries))
	}
	if summaries[0].ID.Hash != h4 {
		t.Errorf("first: got %x, want 04", summaries[0].ID.Hash)
	}
	if summaries[1].ID.Hash != h2 {
		t.Errorf("second: got %x, want 02 (inherited)", summaries[1].ID.Hash)
	}
	if summaries[2].ID.Hash != h1 {
		t.Errorf("third: got %x, want 01 (inherited)", summaries[2].ID.Hash)
	}
}

func TestWalkSnapshotChain_ZeroHash(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()
	summaries, err := WalkSnapshotChain(ctx, store, core.Hash{})
	if err != nil {
		t.Fatalf("WalkSnapshotChain: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for zero hash, got %d", len(summaries))
	}
}

func TestWalkSnapshotChain_BrokenChain(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	// s3 points to a non-existent prev — walk should stop gracefully.
	h3 := core.Hash{3}
	s3 := &core.Snapshot{
		ID:        core.SnapshotID{Hash: h3},
		PrevID:    &core.SnapshotID{Hash: core.Hash{0x99}}, // not stored
		Message:   "orphan",
		Timestamp: 300,
	}
	store.PutSnapshot(ctx, s3)

	summaries, err := WalkSnapshotChain(ctx, store, h3)
	if err != nil {
		t.Fatalf("WalkSnapshotChain: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary (chain breaks at missing prev), got %d", len(summaries))
	}
	if summaries[0].ID.Hash != h3 {
		t.Errorf("got %x, want 03", summaries[0].ID.Hash)
	}
}

func TestResolveCurrentBranchName_DetachedHead(t *testing.T) {
	ctx := context.Background()
	store := memory.NewMemoryStorage()
	// Detached HEAD: SymRef empty, only Target set.
	store.SetRef(ctx, "HEAD", &core.Reference{
		Name:   "HEAD",
		Type:   core.RefTypeHead,
		Target: core.Hash{0x42},
	})
	if name := ResolveCurrentBranchName(ctx, store); name != "" {
		t.Errorf("detached HEAD: expected '', got %q", name)
	}
}

func TestResolveCurrentBranchName_SymRef(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore() // HEAD symref -> heads/main
	if name := ResolveCurrentBranchName(ctx, store); name != "main" {
		t.Errorf("expected 'main', got %q", name)
	}
}

func TestResolveTagTips_SingleTag(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	// One snapshot with one tag pointing at it.
	snap := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{0xaa}},
		Message:   "tagged",
		Timestamp: 100,
	}
	store.PutSnapshot(ctx, snap)
	store.SetRef(ctx, "tags/v1.0", &core.Reference{
		Name:   "tags/v1.0",
		Type:   core.RefTypeTag,
		Target: core.Hash{0xaa},
	})

	result, err := ResolveTagTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveTagTips: %v", err)
	}
	names := result[core.Hash{0xaa}.String()]
	if len(names) != 1 || names[0] != "v1.0" {
		t.Errorf("got %v, want [v1.0]", names)
	}
}

func TestResolveTagTips_MultipleTagsOnSameSnapshot(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	snap := &core.Snapshot{
		ID:        core.SnapshotID{Hash: core.Hash{0xbb}},
		Message:   "release",
		Timestamp: 100,
	}
	store.PutSnapshot(ctx, snap)
	// Tags added out of order to verify alphabetical sorting.
	store.SetRef(ctx, "tags/v2.0", &core.Reference{Name: "tags/v2.0", Type: core.RefTypeTag, Target: core.Hash{0xbb}})
	store.SetRef(ctx, "tags/v1.0", &core.Reference{Name: "tags/v1.0", Type: core.RefTypeTag, Target: core.Hash{0xbb}})
	store.SetRef(ctx, "tags/release-candidate", &core.Reference{Name: "tags/release-candidate", Type: core.RefTypeTag, Target: core.Hash{0xbb}})

	result, err := ResolveTagTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveTagTips: %v", err)
	}
	names := result[core.Hash{0xbb}.String()]
	want := []string{"release-candidate", "v1.0", "v2.0"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("names[%d]: got %q, want %q", i, names[i], w)
		}
	}
}

func TestResolveTagTips_DistinctSnapshots(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()

	s1 := &core.Snapshot{ID: core.SnapshotID{Hash: core.Hash{1}}, Message: "first", Timestamp: 100}
	s2 := &core.Snapshot{ID: core.SnapshotID{Hash: core.Hash{2}}, Message: "second", Timestamp: 200}
	store.PutSnapshot(ctx, s1)
	store.PutSnapshot(ctx, s2)

	store.SetRef(ctx, "tags/alpha", &core.Reference{Name: "tags/alpha", Type: core.RefTypeTag, Target: core.Hash{1}})
	store.SetRef(ctx, "tags/beta", &core.Reference{Name: "tags/beta", Type: core.RefTypeTag, Target: core.Hash{2}})

	result, err := ResolveTagTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveTagTips: %v", err)
	}
	if names := result[core.Hash{1}.String()]; len(names) != 1 || names[0] != "alpha" {
		t.Errorf("s1: got %v, want [alpha]", names)
	}
	if names := result[core.Hash{2}.String()]; len(names) != 1 || names[0] != "beta" {
		t.Errorf("s2: got %v, want [beta]", names)
	}
	// Untagged snapshots must not appear in the map.
	if _, ok := result[core.Hash{9}.String()]; ok {
		t.Errorf("untagged snapshot should have no entry")
	}
}

func TestResolveTagTips_NoTags(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()
	// No tag refs set; result must be a non-nil empty map (callers index it
	// directly, so a nil map would panic on read in Go).
	result, err := ResolveTagTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveTagTips: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestResolveTagTips_ZeroTargetSkipped(t *testing.T) {
	ctx := context.Background()
	store := setupBranchStore()
	// A tag ref with a zero target (shouldn't normally happen, but the
	// function must not panic or surface it as a real entry).
	store.SetRef(ctx, "tags/ghost", &core.Reference{
		Name:   "tags/ghost",
		Type:   core.RefTypeTag,
		Target: core.Hash{},
	})
	result, err := ResolveTagTips(ctx, store)
	if err != nil {
		t.Fatalf("ResolveTagTips: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("zero-target tag should be skipped, got %v", result)
	}
}
