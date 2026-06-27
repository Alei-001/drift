package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const driftAPIReleases = "https://api.github.com/repos/Alei-001/drift/releases"

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func NewUpgradeCmd() *cobra.Command {
	var checkOnly bool

	cmd := &cobra.Command{
		Use:   "upgrade [<version>]",
		Short: "Upgrade drift to the specified version",
		Long: `Download and replace the current drift binary with a GitHub release.

If no version is given, the latest release is used.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			targetVer := ""
			if len(args) > 0 {
				targetVer = strings.TrimPrefix(args[0], "v")
			}
			return runUpgrade(targetVer, checkOnly)
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check", false, "Only check for updates, do not download")
	return cmd
}

func runUpgrade(targetVer string, checkOnly bool) error {
	if version == "dev" {
		return fmt.Errorf("dev build cannot be upgraded (use 'go install github.com/drift/drift/cmd/drift@latest')")
	}

	rel, err := fetchRelease(targetVer)
	if err != nil {
		return err
	}

	relVer := strings.TrimPrefix(rel.TagName, "v")

	if relVer == version {
		fmt.Println(colorGreen(fmt.Sprintf("already at %s", rel.TagName)))
		return nil
	}

	if checkOnly {
		if !versionLess(version, relVer) {
			fmt.Printf("%s → %s (downgrade)\n", colorYellow(version), colorYellow(rel.TagName))
		} else {
			fmt.Printf("%s → %s\n", colorGreen(version), colorGreen(rel.TagName))
		}
		return nil
	}

	if !versionLess(version, relVer) {
		fmt.Printf("%s: %s is newer than %s, downgrading\n", colorYellow("warning"), version, relVer)
	}

	assetURL := findAsset(rel.Assets)
	if assetURL == "" {
		return fmt.Errorf("no binary found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find current binary: %w", err)
	}
	if strings.Contains(exePath, "go-build") {
		return fmt.Errorf("running from go build, self-upgrade unavailable")
	}

	exeDir := filepath.Dir(exePath)

	newPath, err := downloadBinary(exeDir, rel.TagName, assetURL)
	if err != nil {
		return err
	}
	defer os.Remove(newPath)

	if err := replaceBinary(exePath, newPath); err != nil {
		return err
	}

	fmt.Println(colorGreen(fmt.Sprintf("upgraded to %s — restart drift", rel.TagName)))
	return nil
}

func fetchRelease(targetVer string) (*githubRelease, error) {
	url := driftAPIReleases + "/latest"
	if targetVer != "" {
		url = driftAPIReleases + "/tags/v" + targetVer
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "drift/upgrade")
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		if targetVer != "" {
			return nil, fmt.Errorf("release %s not found", targetVer)
		}
		return nil, fmt.Errorf("no releases found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("parse release info: %w", err)
	}
	return &rel, nil
}

func findAsset(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}) string {
	for _, a := range assets {
		if a.Name == "drift.exe" {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func downloadBinary(dir, label, url string) (string, error) {
	matches, _ := filepath.Glob(filepath.Join(dir, "drift_upgrade_*.tmp"))
	for _, m := range matches {
		os.Remove(m)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build download request: %w", err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download binary: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed (HTTP %d)", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(dir, "drift_upgrade_*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	pr := &progressReader{reader: resp.Body, total: resp.ContentLength, label: label}
	if _, err := io.Copy(tmp, pr); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("download binary: %w", err)
	}
	fmt.Println()

	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}

	return tmp.Name(), nil
}

type progressReader struct {
	reader  io.Reader
	total   int64
	current int64
	lastPct int
	label   string
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.current += int64(n)
		if pr.total > 0 {
			pct := int(pr.current * 100 / pr.total)
			if pct != pr.lastPct {
				fmt.Printf("\r  %s  %d%% (%s / %s)    ", colorCyan("downloading "+pr.label), pct, formatSize(pr.current), formatSize(pr.total))
				pr.lastPct = pct
			}
		} else {
			if pr.lastPct == 0 {
				fmt.Printf("\r  %s... ", colorCyan("downloading "+pr.label))
				pr.lastPct = -1
			}
		}
	return n, err
}

func formatSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func replaceBinary(exePath, newPath string) error {
	oldPath := exePath + ".old"

	os.Remove(oldPath)

	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("backup %s → %s: %w", filepath.Base(exePath), filepath.Base(oldPath), err)
	}

	if err := os.Rename(newPath, exePath); err != nil {
		os.Rename(oldPath, exePath)
		return fmt.Errorf("install new binary: %w", err)
	}

	os.Remove(oldPath)
	return nil
}

func versionLess(a, b string) bool {
	ap := parseVersion(a)
	bp := parseVersion(b)
	for i := 0; i < len(ap) && i < len(bp); i++ {
		if ap[i] != bp[i] {
			return ap[i] < bp[i]
		}
	}
	return len(ap) < len(bp)
}

func parseVersion(v string) []int {
	parts := strings.Split(v, ".")
	nums := make([]int, len(parts))
	for i, p := range parts {
		nums[i], _ = strconv.Atoi(p)
	}
	return nums
}
