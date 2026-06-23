package core

import (
	"strings"
	"testing"
)

func TestMyers_Identical(t *testing.T) {
	a := []string{"line1", "line2", "line3"}
	edits := Myers(a, a)
	if len(edits) != 3 {
		t.Fatalf("expected 3 keeps, got %d edits: %+v", len(edits), edits)
	}
	for i, e := range edits {
		if e.Op != DiffKeep {
			t.Errorf("edit[%d] = %c, want keep", i, e.Op)
		}
	}
}

func TestMyers_AllDeleted(t *testing.T) {
	a := []string{"a", "b", "c"}
	edits := Myers(a, nil)
	if len(edits) != 3 {
		t.Fatalf("expected 3 deletes, got %d", len(edits))
	}
	for i, e := range edits {
		if e.Op != DiffDelete {
			t.Errorf("edit[%d] = %c, want delete", i, e.Op)
		}
	}
}

func TestMyers_AllInserted(t *testing.T) {
	edits := Myers(nil, []string{"a", "b", "c"})
	if len(edits) != 3 {
		t.Fatalf("expected 3 inserts, got %d", len(edits))
	}
	for i, e := range edits {
		if e.Op != DiffInsert {
			t.Errorf("edit[%d] = %c, want insert", i, e.Op)
		}
	}
}

func TestMyers_Modified(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "x", "c"}
	edits := Myers(a, b)
	// Expected: keep a, delete b, insert x, keep c
	if len(edits) != 4 {
		t.Fatalf("expected 4 edits, got %d: %+v", len(edits), edits)
	}
	if edits[0].Op != DiffKeep || edits[0].Line != "a" {
		t.Errorf("edit[0] should keep 'a'")
	}
	if edits[1].Op != DiffDelete || edits[1].Line != "b" {
		t.Errorf("edit[1] should delete 'b'")
	}
	if edits[2].Op != DiffInsert || edits[2].Line != "x" {
		t.Errorf("edit[2] should insert 'x'")
	}
	if edits[3].Op != DiffKeep || edits[3].Line != "c" {
		t.Errorf("edit[3] should keep 'c'")
	}
}

func TestMyers_InsertAtFront(t *testing.T) {
	a := []string{"b", "c"}
	b := []string{"a", "b", "c"}
	edits := Myers(a, b)
	if len(edits) != 3 {
		t.Fatalf("expected 3 edits, got %d: %+v", len(edits), edits)
	}
	if edits[0].Op != DiffInsert || edits[0].Line != "a" {
		t.Errorf("edit[0] should insert 'a'")
	}
}

func TestMyers_DeleteAtEnd(t *testing.T) {
	a := []string{"a", "b", "c"}
	b := []string{"a", "b"}
	edits := Myers(a, b)
	if len(edits) != 3 {
		t.Fatalf("expected 3 edits (keep a, keep b, delete c), got %d: %+v", len(edits), edits)
	}
}

func TestMyers_LargeFile(t *testing.T) {
	// Generate 50001 lines — triggers the simpleReplace fallback.
	const n = 50001
	a := make([]string, n)
	b := make([]string, n)
	for i := range a {
		a[i] = "line " + itoa(i)
		b[i] = "line " + itoa(i)
	}
	// Change last line.
	b[n-1] = "modified"
	edits := Myers(a, b)
	if len(edits) == 0 {
		t.Fatal("expected edits for large file")
	}
}

func TestMyers_ModerateFile_NoOOM(t *testing.T) {
	// 10000 identical lines with one small change — Myers should handle
	// this efficiently (not OOM).
	const n = 10000
	a := make([]string, n)
	b := make([]string, n)
	for i := range a {
		a[i] = "common line"
		b[i] = "common line"
	}
	b[n/2] = "different"
	edits := Myers(a, b)
	// Should find the middle snake, produce edits.
	if len(edits) < 2 {
		t.Fatalf("expected at least 2 edits, got %d", len(edits))
	}
}

func TestMyers_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
	}{
		{"empty both", nil, nil},
		{"empty old", nil, []string{"a", "b"}},
		{"empty new", []string{"a", "b"}, nil},
		{"single change", []string{"a", "b", "c"}, []string{"a", "d", "c"}},
		{"append", []string{"a"}, []string{"a", "b", "c"}},
		{"prepend", []string{"c"}, []string{"a", "b", "c"}},
		{"multiple changes", []string{"a", "b", "c", "d"}, []string{"a", "x", "c", "y"}},
		{"all different", []string{"a", "b"}, []string{"x", "y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edits := Myers(tt.a, tt.b)
			// Reconstruct b from a using edits to verify correctness.
			var got []string
			ai := 0
			for _, e := range edits {
				switch e.Op {
				case DiffKeep:
					got = append(got, tt.a[ai])
					ai++
				case DiffDelete:
					ai++
				case DiffInsert:
					got = append(got, e.Line)
				}
			}
			gotStr := strings.Join(got, "\n")
			wantStr := strings.Join(tt.b, "\n")
			if gotStr != wantStr {
				t.Errorf("reconstructed = %q, want %q", gotStr, wantStr)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
