package version

import "testing"

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in     string
		major  int
		minor  int
		patch  int
		pre    []string
		hasErr bool
	}{
		{"v1.2.3", 1, 2, 3, nil, false},
		{"1.2.3", 1, 2, 3, nil, false},
		{"v0.0.1", 0, 0, 1, nil, false},
		{"v1.2.3-alpha.1", 1, 2, 3, []string{"alpha", "1"}, false},
		{"v1.2.3+build.5", 1, 2, 3, nil, false}, // build metadata stripped
		{"v1.2.3-rc.1+build.5", 1, 2, 3, []string{"rc", "1"}, false},
		{"v1.2", 0, 0, 0, nil, true},            // too few parts
		{"v1.2.3.4", 0, 0, 0, nil, true},        // too many parts
		{"v1.2.x", 0, 0, 0, nil, true},          // non-numeric
		{"", 0, 0, 0, nil, true},                // empty
		{"v-1.0.0", 0, 0, 0, nil, true},         // negative
	}
	for _, c := range cases {
		v, err := parseSemver(c.in)
		if c.hasErr {
			if err == nil {
				t.Errorf("parseSemver(%q): expected error, got %+v", c.in, v)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSemver(%q): unexpected error %v", c.in, err)
			continue
		}
		if v.major != c.major || v.minor != c.minor || v.patch != c.patch {
			t.Errorf("parseSemver(%q): got %d.%d.%d, want %d.%d.%d",
				c.in, v.major, v.minor, v.patch, c.major, c.minor, c.patch)
		}
		if !equalStrSlices(v.pre, c.pre) {
			t.Errorf("parseSemver(%q): pre=%v, want %v", c.in, v.pre, c.pre)
		}
	}
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCompareSemver(t *testing.T) {
	// Each row: a < b (expect -1). Swap direction is tested by symmetry.
	less := []struct{ a, b string }{
		{"v1.0.0", "v1.0.1"},
		{"v1.0.0", "v1.1.0"},
		{"v1.0.0", "v2.0.0"},
		{"v1.2.3-alpha", "v1.2.3"},            // pre < release
		{"v1.2.3-alpha.1", "v1.2.3-alpha.2"},  // numeric pre compare
		{"v1.2.3-alpha.9", "v1.2.3-alpha.10"}, // numeric (9 < 10)
		{"v1.2.3-alpha", "v1.2.3-beta"},       // lexical pre compare
		{"v1.2.3-alpha.1", "v1.2.3-beta.1"},   // numeric < non-numeric
		{"v1.2.3-alpha", "v1.2.3-alpha.1"},    // shorter pre < longer pre
		{"v0.9.0", "v1.0.0"},
	}
	for _, c := range less {
		got, err := CompareVersions(c.a, c.b)
		if err != nil {
			t.Fatalf("CompareVersions(%q,%q): %v", c.a, c.b, err)
		}
		if got >= 0 {
			t.Errorf("CompareVersions(%q,%q): got %d, want <0", c.a, c.b, got)
		}
		// Symmetry: b > a.
		got, err = CompareVersions(c.b, c.a)
		if err != nil {
			t.Fatalf("CompareVersions(%q,%q): %v", c.b, c.a, err)
		}
		if got <= 0 {
			t.Errorf("CompareVersions(%q,%q): got %d, want >0", c.b, c.a, got)
		}
	}

	// Equality.
	eq := []string{"v1.2.3", "1.2.3", "v1.2.3+build.1", "v1.2.3+build.2"}
	for _, a := range eq {
		for _, b := range eq {
			got, err := CompareVersions(a, b)
			if err != nil {
				t.Fatalf("CompareVersions(%q,%q): %v", a, b, err)
			}
			if got != 0 {
				t.Errorf("CompareVersions(%q,%q): got %d, want 0", a, b, got)
			}
		}
	}
}

func TestCompareVersions_Invalid(t *testing.T) {
	if _, err := CompareVersions("v1.2.3", "not-a-version"); err == nil {
		t.Error("expected error for invalid version")
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate string
		current   string
		want      bool
		wantErr   bool
	}{
		{"v1.0.0", "v0.9.0", true, false},
		{"v0.9.0", "v1.0.0", false, false},
		{"v1.0.0", "v1.0.0", false, false},
		{"v1.0.0", "(devel)", true, false},  // dev is always older
		{"v1.0.0", "", true, false},          // empty current treated as dev
		{"not-a-version", "v1.0.0", false, true},
		{"v1.0.0", "not-a-version", false, true},
	}
	for _, c := range cases {
		got, err := IsNewer(c.candidate, c.current)
		if c.wantErr {
			if err == nil {
				t.Errorf("IsNewer(%q,%q): expected error", c.candidate, c.current)
			}
			continue
		}
		if err != nil {
			t.Errorf("IsNewer(%q,%q): unexpected error %v", c.candidate, c.current, err)
			continue
		}
		if got != c.want {
			t.Errorf("IsNewer(%q,%q): got %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}
