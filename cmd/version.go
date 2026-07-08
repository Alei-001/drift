package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/Alei-001/drift/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show drift version and build info",
	Long:  "Show the version, commit, build date, and platform of the drift binary. This command does not require a drift repository and can be run anywhere.",
	Args:  cobra.NoArgs,
	RunE:  runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// versionData is the JSON data payload of `drift version`.
type versionData struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Built     string `json:"built"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

func runVersion(cmd *cobra.Command, args []string) error {
	info := version.GetInfo()

	if globalJSON {
		return outputJSON(JSONEnvelope{
			Command: "version",
			Status:  "ok",
			Data: versionData{
				Version:   info.Version,
				Commit:    info.Commit,
				Built:     info.Built,
				GoVersion: info.GoVersion,
				OS:        info.OS,
				Arch:      info.Arch,
			},
		})
	}

	// Quiet mode: still emit the bare version string so `drift -q version`
	// is script-friendly (one line, no decoration).
	if globalQuiet {
		fmt.Println(info.Version)
		return nil
	}

	// Human-readable: two lines.
	fmt.Println(info.String())
	fmt.Printf("  %s  %s/%s\n", info.GoVersion, runtime.GOOS, runtime.GOARCH)
	return nil
}
