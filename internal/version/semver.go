package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// semver represents a parsed semantic version (major.minor.patch) with an
// optional pre-release suffix. It is intentionally minimal: drift release
// tags follow "vMAJOR.MINOR.PATCH" with optional "-pre.N" pre-release
// identifiers. Build metadata ("+...") is ignored for comparison, matching
// the SemVer spec.
type semver struct {
	major, minor, patch int
	pre                 []string // nil/empty means a release (higher than any pre)
}

// errInvalidVersion is returned when a version string is not a recognizable
// semantic version. Callers should treat this as "cannot compare".
var errInvalidVersion = errors.New("invalid semantic version")

// parseSemver parses a version string like "v1.2.3", "1.2.3", "v1.2.3-alpha.1".
// A leading 'v' is optional. Returns errInvalidVersion for anything that does
// not match the expected shape.
func parseSemver(s string) (semver, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return semver{}, errInvalidVersion
	}

	// Strip build metadata ("+...") — not used for comparison.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre = s[i+1:]
		s = s[:i]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, errInvalidVersion
	}
	v := semver{}
	var err error
	if v.major, err = strconv.Atoi(parts[0]); err != nil {
		return semver{}, errInvalidVersion
	}
	if v.minor, err = strconv.Atoi(parts[1]); err != nil {
		return semver{}, errInvalidVersion
	}
	if v.patch, err = strconv.Atoi(parts[2]); err != nil {
		return semver{}, errInvalidVersion
	}
	if v.major < 0 || v.minor < 0 || v.patch < 0 {
		return semver{}, errInvalidVersion
	}
	if pre != "" {
		v.pre = strings.Split(pre, ".")
	}
	return v, nil
}

// compareSemver returns -1, 0, or 1 according to SemVer precedence rules.
// A release version (no pre-release) is greater than any pre-release of the
// same major.minor.patch. Pre-release identifiers are compared per the spec:
// numeric identifiers numerically, others lexically, numeric < non-numeric.
func compareSemver(a, b semver) int {
	if a.major != b.major {
		return sign(a.major - b.major)
	}
	if a.minor != b.minor {
		return sign(a.minor - b.minor)
	}
	if a.patch != b.patch {
		return sign(a.patch - b.patch)
	}
	// Release (no pre) > pre-release.
	aRel := len(a.pre) == 0
	bRel := len(b.pre) == 0
	if aRel && bRel {
		return 0
	}
	if aRel {
		return 1
	}
	if bRel {
		return -1
	}
	return comparePreRelease(a.pre, b.pre)
}

// comparePreRelease compares two pre-release identifier lists per SemVer.
func comparePreRelease(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		ai, aErr := strconv.Atoi(a[i])
		bi, bErr := strconv.Atoi(b[i])
		switch {
		case aErr == nil && bErr == nil:
			if ai != bi {
				return sign(ai - bi)
			}
		case aErr == nil:
			// numeric identifiers have lower precedence than non-numeric.
			return -1
		case bErr == nil:
			return 1
		default:
			if a[i] != b[i] {
				if a[i] < b[i] {
					return -1
				}
				return 1
			}
		}
	}
	// All shared identifiers equal: shorter list has lower precedence.
	return sign(len(a) - len(b))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// CompareVersions compares two version strings. Returns -1, 0, 1 if a is
// less than, equal to, or greater than b. Returns an error wrapping
// errInvalidVersion if either string is not a recognizable semver.
func CompareVersions(a, b string) (int, error) {
	sa, err := parseSemver(a)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", errInvalidVersion, a)
	}
	sb, err := parseSemver(b)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", errInvalidVersion, b)
	}
	return compareSemver(sa, sb), nil
}

// IsNewer reports whether candidate is strictly newer than current. A
// development version ("(devel)") for current is treated as older than any
// real release, so `drift upgrade` can move from a dev build to a release.
func IsNewer(candidate, current string) (bool, error) {
	if current == "(devel)" || current == "" {
		// A dev build has no version to compare; any real release is newer.
		if _, err := parseSemver(candidate); err != nil {
			return false, err
		}
		return true, nil
	}
	c, err := CompareVersions(candidate, current)
	if err != nil {
		return false, err
	}
	return c > 0, nil
}
