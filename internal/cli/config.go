package cli

import (
	"fmt"
	"sort"

	"github.com/drift/drift/internal/config"
	"github.com/spf13/cobra"
)

var (
	configList   bool
	configUnset  bool
)

var configCmd = &cobra.Command{
	Use:   "config <key> [value]",
	Short: "Get or set configuration options",
	Long: `Get or set configuration options.

Examples:
  drift config user.name "Jane Doe"
  drift config user.email "jane@example.com"
  drift config user.name
  drift config --list
  drift config --unset user.email`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// --list: print all config values.
		if configList {
			printConfigAll(sharedConfig)
			return nil
		}

		// --unset <key>: reset a config value to its default/empty.
		if configUnset {
			if len(args) == 0 {
				return fmt.Errorf("--unset requires a key")
			}
			if err := unsetConfigValue(sharedConfig, args[0]); err != nil {
				return err
			}
			if err := config.SaveConfig(sharedStore.DriftDir(), sharedConfig); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			fmt.Printf("Unset %s\n", args[0])
			return nil
		}

		if len(args) == 0 {
			return cmd.Help()
		}

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
		fmt.Printf("Set %s to %s\n", key, value)
		return nil
	},
}

func init() {
	configCmd.Flags().BoolVar(&configList, "list", false, "List all configuration values")
	configCmd.Flags().BoolVar(&configUnset, "unset", false, "Remove a configuration value")
	rootCmd.AddCommand(configCmd)
}

// configKeyInfo pairs a config key with its current value.
type configKeyInfo struct {
	Key   string
	Value string
}

// listConfigEntries returns all known config keys with their current values.
func listConfigEntries(cfg *config.Config) []configKeyInfo {
	return []configKeyInfo{
		{"user.name", cfg.User.Name},
		{"user.email", cfg.User.Email},
		{"core.default_branch", cfg.Core.DefaultBranch},
		{"core.autocrlf", cfg.Core.AutoCRLF},
	}
}

func printConfigAll(cfg *config.Config) {
	entries := listConfigEntries(cfg)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	for _, e := range entries {
		fmt.Printf("%s=%s\n", e.Key, e.Value)
	}
}

func getConfigValue(cfg *config.Config, key string) (string, error) {
	switch key {
	case "user.name":
		return cfg.User.Name, nil
	case "user.email":
		return cfg.User.Email, nil
	case "core.default_branch":
		return cfg.Core.DefaultBranch, nil
	case "core.autocrlf":
		return cfg.Core.AutoCRLF, nil
	default:
		return "", fmt.Errorf("unknown config key: %s (supported: user.name, user.email, core.default_branch, core.autocrlf)", key)
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
	case "core.autocrlf":
		cfg.Core.AutoCRLF = value
	default:
		return fmt.Errorf("unknown config key: %s (supported: user.name, user.email, core.default_branch, core.autocrlf)", key)
	}
	return nil
}

// unsetConfigValue resets a config key to its empty/default value.
func unsetConfigValue(cfg *config.Config, key string) error {
	switch key {
	case "user.name":
		cfg.User.Name = ""
	case "user.email":
		cfg.User.Email = ""
	case "core.default_branch":
		cfg.Core.DefaultBranch = "main"
	case "core.autocrlf":
		cfg.Core.AutoCRLF = ""
	default:
		return fmt.Errorf("unknown config key: %s (supported: user.name, user.email, core.default_branch, core.autocrlf)", key)
	}
	return nil
}
