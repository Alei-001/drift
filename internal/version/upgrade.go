package version

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
)

// Result summarizes the outcome of an Upgrade call.
type Result struct {
	FromVersion string
	ToVersion   string
	Upgraded    bool   // true when the binary was actually replaced
	Message     string // human-readable detail
}

// UpgradeOptions controls the behaviour of Upgrade.
type UpgradeOptions struct {
	Check          bool      // only check for a newer release, do not replace the binary
	Force          bool      // replace even when already up to date
	APIURL         string    // override GitHub API base (for tests); defaults to https://api.github.com
	ProgressWriter io.Writer // optional progress bar output (nil = no progress)
}

// Upgrade checks for a newer drift release on GitHub and, when available,
// downloads the matching binary and atomically replaces the running
// executable. Set Check=true to only report the latest version without
// modifying anything. Set Force=true to reinstall the same version.
//
// On success Result.Upgraded is true when the binary was replaced, false
// when already up to date (or Check-only). A non-nil error is returned for
// network, release-matching, download, checksum, extraction, or replacement
// failures; the CLI maps these to user-facing hints via errors.Is.
func Upgrade(ctx context.Context, currentVersion string, opt UpgradeOptions) (Result, error) {
	if opt.APIURL == "" {
		opt.APIURL = "https://api.github.com"
	}
	res := Result{FromVersion: currentVersion}

	rel, err := latestRelease(ctx, opt.APIURL)
	if err != nil {
		return res, err
	}
	res.ToVersion = rel.TagName

	newer, err := IsNewer(rel.TagName, currentVersion)
	if err != nil {
		return res, &upgradeError{kind: "network", err: err}
	}
	if !newer && !opt.Force {
		res.Upgraded = false
		res.Message = fmt.Sprintf("already up to date (%s)", rel.TagName)
		return res, nil
	}
	if opt.Check {
		res.Upgraded = false
		res.Message = fmt.Sprintf("update available: %s -> %s", currentVersion, rel.TagName)
		return res, nil
	}

	asset, err := matchAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return res, err
	}

	archivePath, err := download(ctx, asset.BrowserDownloadURL, opt.ProgressWriter)
	if err != nil {
		return res, err
	}
	defer os.Remove(archivePath)

	// Optional checksum verification.
	if cs, ok := findChecksumAsset(rel.Assets); ok {
		csPath, derr := download(ctx, cs.BrowserDownloadURL, nil)
		if derr != nil {
			return res, derr
		}
		defer os.Remove(csPath)
		ok, verr := verifyChecksum(csPath, asset.Name, archivePath)
		if verr != nil {
			// Checksum file malformed: warn but proceed (HTTPS protects transport).
			res.Message = "checksum file unreadable, skipping verification"
		} else if !ok {
			return res, &upgradeError{kind: "checksum", err: errors.New("checksum mismatch")}
		}
	}

	newBin, err := os.CreateTemp("", "drift-new-*")
	if err != nil {
		return res, &upgradeError{kind: "replace", err: err}
	}
	newBinPath := newBin.Name()
	newBin.Close()
	defer os.Remove(newBinPath)

	if _, err := extractBinary(archivePath, newBinPath, runtime.GOOS); err != nil {
		return res, err
	}

	if err := replaceExecutable(newBinPath); err != nil {
		// errOldBinaryRemovableLater means the new binary is in place but
		// the old one could not be deleted (common on Windows where the
		// running process still holds a handle to the renamed file). This
		// is a successful upgrade with a cleanup caveat, not a failure.
		if !errors.Is(err, errOldBinaryRemovableLater) {
			return res, &upgradeError{kind: "replace", err: err}
		}
		res.Upgraded = true
		res.Message = fmt.Sprintf("%s -> %s (old binary kept as .old, will be removed on next run)", currentVersion, rel.TagName)
		return res, nil
	}

	res.Upgraded = true
	res.Message = fmt.Sprintf("%s -> %s", currentVersion, rel.TagName)
	return res, nil
}
