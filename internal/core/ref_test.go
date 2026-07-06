package core

import "testing"

func TestRefType_Values(t *testing.T) {
	// RefType is a string type; verify the expected constants.
	if RefTypeBranch != "branch" {
		t.Errorf("RefTypeBranch = %q, want %q", RefTypeBranch, "branch")
	}
	if RefTypeTag != "tag" {
		t.Errorf("RefTypeTag = %q, want %q", RefTypeTag, "tag")
	}
	if RefTypeHead != "HEAD" {
		t.Errorf("RefTypeHead = %q, want %q", RefTypeHead, "HEAD")
	}
}

func TestReference_Fields(t *testing.T) {
	r := Reference{
		Name:   "heads/main",
		Type:   RefTypeBranch,
		Target: Hash{0x01, 0x02},
		SymRef: "",
	}
	if r.Name != "heads/main" {
		t.Errorf("Name: got %q, want %q", r.Name, "heads/main")
	}
	if r.Type != RefTypeBranch {
		t.Errorf("Type: got %q, want %q", r.Type, RefTypeBranch)
	}
	if r.Target != (Hash{0x01, 0x02}) {
		t.Errorf("Target: got %v, want %v", r.Target, Hash{0x01, 0x02})
	}
	if r.SymRef != "" {
		t.Errorf("SymRef: got %q, want empty", r.SymRef)
	}
}

func TestReference_SymRef(t *testing.T) {
	r := Reference{
		Name:   "HEAD",
		Type:   RefTypeHead,
		SymRef: "heads/main",
	}
	if r.SymRef != "heads/main" {
		t.Errorf("SymRef: got %q, want %q", r.SymRef, "heads/main")
	}
	if r.Type != RefTypeHead {
		t.Errorf("Type: got %q, want %q", r.Type, RefTypeHead)
	}
}

func TestReference_ZeroTarget(t *testing.T) {
	r := Reference{Name: "heads/main", Type: RefTypeBranch}
	if !r.Target.IsZero() {
		t.Error("expected zero Target for new branch")
	}
}
