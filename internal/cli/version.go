package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is set at build time via ldflags:
//   go build -ldflags "-X github.com/drift/drift/internal/cli.version=0.1.0" ./cmd/drift/
// When not set (e.g. go run / go test), it defaults to "dev".
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show drift version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("drift %s\n", version)
		return nil
	},
}
