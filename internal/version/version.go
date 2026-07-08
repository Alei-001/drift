// Package version exposes the build-time version metadata of the drift
// binary and the self-upgrade workflow.
//
// The Version, Commit and BuildDate variables are intended to be set via
// -ldflags at build time:
//
//	go build -ldflags "\
//	  -X github.com/Alei-001/drift/internal/version.Version=v0.1.0 \
//	  -X github.com/Alei-001/drift/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/Alei-001/drift/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  ./cmd/drift
//
// When unset (e.g. a plain `go build` or `go install` without ldflags), Info
// falls back to runtime/debug.ReadBuildInfo, which for `go install`-built
// binaries carries the module version and VCS revision.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Build-time variables. Overridable via -ldflags "-X ...=value".
var (
	Version   = "(devel)"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info describes the running binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Built     string `json:"built"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// GetInfo returns the version metadata of the running binary, enriched with
// the toolchain and platform. When the ldflags-injected Version is the
// default "(devel)" placeholder, it is replaced with the module version
// reported by runtime/debug.ReadBuildInfo (if any) so that `go install`-built
// binaries still report a meaningful version.
func GetInfo() Info {
	info := Info{
		Version:   Version,
		Commit:    Commit,
		Built:     BuildDate,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}

	// Enrich from build info when the ldflags values were not injected.
	if info.Version == "(devel)" || info.Commit == "unknown" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if info.Version == "(devel)" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				info.Version = bi.Main.Version
			}
			if info.Commit == "unknown" {
				if rev := vcsRevision(bi); rev != "" {
					info.Commit = rev
				}
			}
		}
	}
	return info
}

// vcsRevision extracts the short VCS revision from build info settings, if
// present. `go install` and builds inside a VCS tree populate vcs.revision.
func vcsRevision(bi *debug.BuildInfo) string {
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			if len(s.Value) > 7 {
				return s.Value[:7]
			}
			return s.Value
		}
	}
	return ""
}

// String returns a single-line human-readable summary, e.g.
// "drift v0.1.0 (commit: a1b2c3d, built: 2026-07-08T12:00:00Z)".
func (i Info) String() string {
	return fmt.Sprintf("drift %s (commit: %s, built: %s)", i.Version, i.Commit, i.Built)
}

// Platform returns the "os/arch" string used to match release assets.
func (i Info) Platform() string {
	return i.OS + "/" + i.Arch
}

// IsDevel reports whether the binary is an unreleased development build.
func (i Info) IsDevel() bool {
	return i.Version == "(devel)" || strings.HasPrefix(i.Version, "v0.0.0")
}
