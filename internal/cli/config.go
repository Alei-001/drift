package cli

import (
	"fmt"

	"github.com/drift/drift/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config <key> [value]",
	Short: "Get or set configuration options",
	Long: `Get or set configuration options.

Examples:
  drift config user.name "Jane Doe"
  drift config user.email "jane@example.com"
  drift config user.name`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		if len(args) == 1 {
			// Get mode.
			val, err := getConfigValue(sharedConfig, key)
			if err != nil {
				return err
			}
			fmt.Println(val)
			return nil
		}

		// Set mode.
		value := args[1]
		if err := setConfigValue(sharedConfig, key, value); err != nil {
			return err
		}
		if err := config.SaveConfig(sharedStore.DriftDir(), sharedConfig); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func getConfigValue(cfg *config.Config, key string) (string, error) {
	switch key {
	case "user.name":
		return cfg.User.Name, nil
	case "user.email":
		return cfg.User.Email, nil
	case "core.default_branch":
		return cfg.Core.DefaultBranch, nil
	default:
		return "", fmt.Errorf("unknown config key: %s (supported: user.name, user.email, core.default_branch)", key)
	}
}

func setConfigValue(cfg *config.Config, key, value string) error {
	switch key {
	case "user.name":
		cfg.User.Name = value
	case "user.email":
		cfg.User.Email = value
	case "core.default_branch":
		cfg.Core.DefaultBranch = value
	default:
		return fmt.Errorf("unknown config key: %s (supported: user.name, user.email, core.default_branch)", key)
	}
	return nil
}
