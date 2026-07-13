package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// repoOwner and repoName identify the GitHub repository that publishes drift
// releases. They are package-level variables so tests can override them to
// point at a fake server.
var (
	repoOwner = "Alei-001"
	repoName  = "drift"
)

// httpTimeout is the per-request timeout for release API and asset downloads.
const httpTimeout = 60 * time.Second

// Release describes the subset of GitHub release metadata drift consumes.
type Release struct {
	TagName string  `json:"tag_name"`
	Name    string  `json:"name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// Asset describes a single downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// upgradeError wraps a failure with a stable kind so the CLI can format a
// helpful hint without string matching on the error message.
type upgradeError struct {
	kind string // "network" | "no-release" | "no-asset" | "download" | "checksum" | "extract" | "replace"
	err  error
}

func (e *upgradeError) Error() string {
	if e.err == nil {
		return e.kind
	}
	return fmt.Sprintf("%s: %v", e.kind, e.err)
}

func (e *upgradeError) Unwrap() error { return e.err }

// ErrNetwork, ErrNoRelease, ErrNoAsset, ErrChecksumMismatch are sentinel-like
// errors returned by Upgrade. The CLI inspects them with errors.Is to tailor
// the user hint. ErrChecksumMismatch is wrapped by the checksum verification
// path both when the published checksum does not match the downloaded asset
// and when the checksums file is malformed and cannot be parsed — both cases
// mean verification failed and the upgrade must be aborted (fail-closed).
var (
	ErrNetwork          = errors.New("network error")
	ErrNoRelease        = errors.New("no release available")
	ErrNoAsset          = errors.New("no matching release asset")
	ErrChecksumMismatch = errors.New("checksum verification failed")
)

// latestRelease fetches the latest release from the GitHub releases API.
// owner/name/apiURL may be overridden (for tests).
func latestRelease(ctx context.Context, apiURL string) (Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiURL, repoOwner, repoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, &upgradeError{kind: "network", err: err}
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, &upgradeError{kind: "network", err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return Release{}, &upgradeError{kind: "no-release", err: ErrNoRelease}
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, &upgradeError{kind: "network", err: fmt.Errorf("%w: github api returned %s", ErrNetwork, resp.Status)}
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, &upgradeError{kind: "network", err: err}
	}
	return rel, nil
}

// matchAsset selects the release asset for the current GOOS/GOARCH, or
// returns ErrNoAsset when none matches. Naming convention:
//
//	drift_<version>_<os>_<arch>.{zip|tar.gz}
//
// windows builds are packaged as .zip; all others as .tar.gz.
func matchAsset(assets []Asset, goos, goarch string) (Asset, error) {
	wantExt := ".tar.gz"
	if goos == "windows" {
		wantExt = ".zip"
	}
	want := fmt.Sprintf("_%s_%s%s", goos, goarch, wantExt)
	for _, a := range assets {
		if strings.HasSuffix(a.Name, want) {
			return a, nil
		}
	}
	return Asset{}, &upgradeError{kind: "no-asset", err: fmt.Errorf("%w for %s/%s", ErrNoAsset, goos, goarch)}
}

// findChecksumAsset returns the checksums asset (drift_<version>_checksums.txt)
// if present, otherwise an empty Asset and ok=false.
func findChecksumAsset(assets []Asset) (Asset, bool) {
	for _, a := range assets {
		if strings.HasSuffix(a.Name, "_checksums.txt") {
			return a, true
		}
	}
	return Asset{}, false
}
