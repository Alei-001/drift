package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/version"
)

var (
	upgradeCheck bool
	upgradeForce bool
	// upgradeAPIURL is the GitHub API base URL. Defaults to the real endpoint;
	// tests override it to point at an httptest server.
	upgradeAPIURL = "https://api.github.com"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade drift to the latest release",
	Long: `Check GitHub for a newer drift release and, when available, download the
binary for the current platform and atomically replace the running
executable. The previous binary is preserved as "<exe>.old" until the next
upgrade so a failed install can be rolled back.

This command does not require a drift repository and can be run anywhere.
Use --check to only report the latest available version without modifying
the binary. Use --force to reinstall even when already up to date.

Release assets follow the naming convention
  drift_<version>_<os>_<arch>.{zip|tar.gz}
with an optional drift_<version>_checksums.txt (SHA-256) that is verified
when present.`,
	Args: cobra.NoArgs,
	RunE: runUpgrade,
}

func init() {
	upgradeCmd.Flags().BoolVar(&upgradeCheck, "check", false, "only check for a newer release, do not upgrade")
	upgradeCmd.Flags().BoolVar(&upgradeForce, "force", false, "reinstall even when already up to date")
	rootCmd.AddCommand(upgradeCmd)
}

// upgradeData is the JSON data payload of `drift upgrade`.
type upgradeData struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Upgraded bool   `json:"upgraded"`
	Message  string `json:"message"`
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	if cmd != nil {
		ctx = cmd.Context()
	}
	info := version.GetInfo()
	current := info.Version

	opt := version.UpgradeOptions{
		Check:  upgradeCheck,
		Force:  upgradeForce,
		APIURL: upgradeAPIURL,
	}
	if !globalJSON && !globalQuiet {
		opt.ProgressWriter = os.Stderr
	}

	// Warn (human mode) when running a development build: version comparison
	// treats dev as older than any release, so an upgrade is always offered.
	if info.IsDevel() && !globalJSON {
		statusWarn("Running a development build (%s); latest release will be offered.", current)
	}

	res, err := version.Upgrade(ctx, current, opt)
	if err != nil {
		upgradeReportFailed(err)
		return ErrSilent
	}

	if globalJSON {
		return outputJSON(JSONEnvelope{
			Command: "upgrade",
			Status:  "ok",
			Data: upgradeData{
				From:     res.FromVersion,
				To:       res.ToVersion,
				Upgraded: res.Upgraded,
				Message:  res.Message,
			},
		})
	}
	if globalQuiet {
		return nil
	}

	if !res.Upgraded {
		statusOK("Upgrade")
		fmt.Printf("  %s\n", res.Message)
		return nil
	}

	statusOK("Upgrade")
	fmt.Printf("  %s\n", res.Message)
	fmt.Println("  restart drift to use the new version.")
	return nil
}

// upgradeReportFailed maps a version.Upgrade error to a user-facing failure
// report (JSON-aware) with a tailored hint. The error kind is classified via
// errors.Is so no string matching on messages is used.
func upgradeReportFailed(err error) {
	hint := ""
	switch {
	case errors.Is(err, version.ErrNetwork):
		hint = "check your network connection and GitHub availability."
	case errors.Is(err, version.ErrNoRelease):
		hint = "no GitHub release has been published yet; see https://github.com/Alei-001/drift/releases."
	case errors.Is(err, version.ErrNoAsset):
		hint = "no prebuilt binary for this platform; build from source with 'go install github.com/Alei-001/drift/cmd/drift@latest'."
	default:
		hint = "see https://github.com/Alei-001/drift/releases for manual download."
	}
	reportFailed("Upgrade", "upgrade", err.Error(), hint)
}
